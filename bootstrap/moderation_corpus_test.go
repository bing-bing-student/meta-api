package bootstrap

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"

	"meta-api/app/service/comment/moderation"
)

var moderationTargetCategories = []string{
	"abuse", "contact", "decoded_url", "drugs", "gambling", "gore", "harmful_value", "hate",
	"illegal_privacy", "minor", "political", "risk_phrase", "script_injection", "sensitive", "sexual",
	"spam_fraud", "text_quality", "url", "violence",
}

// TestModerationDomainCorpus 验证 19 个审核领域的定向正反例。
// 每个领域必须恰好包含 5 条正常样本和 5 条风险样本，防止新增规则只验证拦截、不验证误杀。
func TestModerationDomainCorpus(t *testing.T) {
	root := moderationRepositoryRoot(t)
	engine := moderationCorpusEngine(t, root)
	stats, failures := verifyModerationCorpus(t, engine,
		filepath.Join(root, "app/service/comment/testdata/domain_targeted.tsv"))

	if len(failures) > 0 {
		t.Fatalf("%d/%d failures:\n%s", len(failures), stats.total, strings.Join(failures, "\n"))
	}
	if stats.total != len(moderationTargetCategories)*10 {
		t.Fatalf("targeted corpus contains %d samples, want %d", stats.total, len(moderationTargetCategories)*10)
	}
	for _, category := range moderationTargetCategories {
		counts := stats.categories[category]
		if counts["approved"] != 5 || counts["risk"] != 5 {
			t.Errorf("category %s counts = %+v, want approved=5 risk=5", category, counts)
		}
	}
}

// TestModerationRelationAdversarialCorpus 验证词面高度相似但关系立场相反的镜像样本。
// 输入固定为 20 条正常评论和 20 条风险评论，重点防止否定、引用及跨分句承接发生错绑。
func TestModerationRelationAdversarialCorpus(t *testing.T) {
	root := moderationRepositoryRoot(t)
	engine := moderationCorpusEngine(t, root)
	stats, failures := verifyModerationCorpus(t, engine,
		filepath.Join(root, "app/service/comment/testdata/relation_adversarial.tsv"))

	if len(failures) > 0 {
		t.Fatalf("%d/%d failures:\n%s", len(failures), stats.total, strings.Join(failures, "\n"))
	}
	approved, risk := 0, 0
	for _, counts := range stats.categories {
		approved += counts["approved"]
		risk += counts["risk"]
	}
	if stats.total != 40 || approved != 20 || risk != 20 {
		t.Fatalf("adversarial corpus counts: total=%d approved=%d risk=%d, want 40/20/20",
			stats.total, approved, risk)
	}
}

// TestModerationSexualEvasionCorpus 验证色情邀约、福利引流、身体隐私索要及其变体。
// 输入固定为 28 条风险评论和 25 条医疗、治理、反诈等正常对照。
func TestModerationSexualEvasionCorpus(t *testing.T) {
	root := moderationRepositoryRoot(t)
	engine := moderationCorpusEngine(t, root)
	stats, failures := verifyModerationCorpus(t, engine,
		filepath.Join(root, "app/service/comment/testdata/sexual_evasion.tsv"))

	if len(failures) > 0 {
		t.Fatalf("%d/%d failures:\n%s", len(failures), stats.total, strings.Join(failures, "\n"))
	}
	approved, risk := 0, 0
	for _, counts := range stats.categories {
		approved += counts["approved"]
		risk += counts["risk"]
	}
	if stats.total != 53 || approved != 25 || risk != 28 {
		t.Fatalf("sexual evasion corpus counts: total=%d approved=%d risk=%d, want 53/25/28",
			stats.total, approved, risk)
	}
}

// TestModerationSequentialActionScope 验证顺序连接词会开启新的动作作用域。
// 每条输入的前半句、后半句含义不同；输出必须保留两个分句，并在第二个分句
// 建立未被前句否定或治理立场覆盖的色情行为关系。
func TestModerationSequentialActionScope(t *testing.T) {
	root := moderationRepositoryRoot(t)
	engine := moderationCorpusEngine(t, root)
	tests := []string{
		"不要在评论区加联系方式，然后进行luoliao",
		"禁止在评论区加联系方式，然后进行luoliao",
		"我们可以在评论区加联系方式，然后luoliao",
	}
	for _, content := range tests {
		t.Run(content, func(t *testing.T) {
			result := engine.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
			if result.Status == "approved" {
				t.Fatalf("sequential sexual action was approved: reasons=%v trace=%+v", result.Reasons, result.Trace)
			}
			if len(result.Trace.Clauses) != 2 {
				t.Fatalf("semantic clauses=%d, want 2: %+v", len(result.Trace.Clauses), result.Trace.Clauses)
			}
			if result.Trace.DecisionEngine == nil {
				t.Fatal("decision engine trace is missing")
			}
			found := false
			relationAction, relationEvidence := "", ""
			for _, relation := range result.Trace.DecisionEngine.Context.Relations {
				if relation.Clause == 2 && relation.Category == "sexual" && relation.Object == "裸聊" &&
					relation.Action != "" && !relation.Negated && !relation.Quoted && !relation.Reported {
					found = true
					relationAction = relation.Action
					relationEvidence = relation.Evidence
					break
				}
			}
			if !found {
				t.Fatalf("second clause has no actionable sexual relation: candidates=%+v relations=%+v",
					result.Trace.DecisionEngine.Candidates, result.Trace.DecisionEngine.Context.Relations)
			}
			t.Logf("status=%s score=%d sexual_relation=%s evidence=%s",
				result.Status, result.Score, relationAction, relationEvidence)
		})
	}
}

// TestModerationGoldenCorpus 验证全部人工与生成语料。
// 全量串行执行约需两分钟，默认跳过；策略发布前设置 MODERATION_FULL_CORPUS=1 显式运行。
func TestModerationGoldenCorpus(t *testing.T) {
	if os.Getenv("MODERATION_FULL_CORPUS") != "1" {
		t.Skip("set MODERATION_FULL_CORPUS=1 before a policy release")
	}
	root := moderationRepositoryRoot(t)
	engine := moderationCorpusEngine(t, root)
	files := []string{
		"normal.tsv",
		"violation.tsv",
		"normal_blog_generated.tsv",
		"violation_blog_generated.tsv",
		"domain_targeted.tsv",
		"relation_adversarial.tsv",
		"sexual_evasion.tsv",
	}
	total := 0
	failures := make([]string, 0)
	for _, name := range files {
		stats, corpusFailures := verifyModerationCorpus(t, engine,
			filepath.Join(root, "app/service/comment/testdata", name))
		total += stats.total
		failures = append(failures, corpusFailures...)
	}
	if len(failures) > 0 {
		t.Fatalf("%d/%d failures:\n%s", len(failures), total, strings.Join(failures, "\n"))
	}
	t.Logf("verified %d moderation samples", total)
}

type moderationCorpusStats struct {
	total      int
	categories map[string]map[string]int
}

func moderationRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve corpus test file")
	}
	return filepath.Dir(filepath.Dir(currentFile))
}

func moderationCorpusEngine(t *testing.T, root string) *moderation.Moderator {
	t.Helper()
	cfg, _, err := loadConfigFileSet([]string{
		filepath.Join(root, "config/app.yml"),
		filepath.Join(root, "config/rate_limit.yml"),
		filepath.Join(root, "config/comment_moderation.manifest.yml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return moderation.NewModerator(cfg, zap.NewNop(), nil)
}

func verifyModerationCorpus(t *testing.T, engine *moderation.Moderator, path string) (moderationCorpusStats, []string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 6
	if _, err = reader.Read(); err != nil {
		t.Fatal(err)
	}
	stats := moderationCorpusStats{categories: make(map[string]map[string]int)}
	failures := make([]string, 0)
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		expected := row[2]
		category := row[3]
		if stats.categories[category] == nil {
			stats.categories[category] = make(map[string]int)
		}
		stats.categories[category][expected]++
		stats.total++

		result := engine.ModerateWithBehavior(context.Background(), moderation.Request{Content: row[1]}, nil)
		matched := expected == "approved" && result.Status == "approved" ||
			expected == "risk" && result.Status != "approved" ||
			expected != "approved" && expected != "risk" && result.Status == expected
		if !matched {
			intent := ""
			if result.Trace.DecisionEngine != nil {
				intent = result.Trace.DecisionEngine.Context.Intent
			}
			failures = append(failures, fmt.Sprintf(
				"%s/%s category=%s expected=%s actual=%s score=%d intent=%s reasons=%s text=%s clauses=%+v candidates=%+v evidence=%+v relations=%+v",
				filepath.Base(path), row[0], category, expected, result.Status, result.Score, intent,
				strings.Join(result.Reasons, ";"), row[1], result.Trace.Clauses,
				result.Trace.DecisionEngine.Candidates, result.Trace.DecisionEngine.Evidence,
				result.Trace.DecisionEngine.Context.Relations,
			))
		}
	}
	return stats, failures
}
