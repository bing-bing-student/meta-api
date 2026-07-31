package comment

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	"meta-api/app/service/comment/moderation"
	appconfig "meta-api/config"
)

const (
	moderationRegressionDir        = "testdata/comment_moderation"
	maxFalsePositiveRate           = 0
	maxFalseNegativeRate           = 0
	moderationRegressionReportFile = "report.txt"
)

type moderationCorpusCase struct {
	ID       string
	Text     string
	Expected string
	Category string
	Tags     string
	Note     string
	Corpus   string
}

type moderationCorpusResult struct {
	Case   moderationCorpusCase
	Result moderation.Result
	Wrong  bool
	Strict bool
}

type moderationCorpusMetrics struct {
	Total              int
	NormalTotal        int
	ViolationTotal     int
	GrayTotal          int
	Approved           int
	Pending            int
	Rejected           int
	FalsePositive      int
	FalseNegative      int
	StrictMismatch     int
	GrayMismatch       int
	FalsePositiveRate  float64
	FalseNegativeRate  float64
	CategoryWrongCount map[string]int
}

func TestCommentModerationGoldenCorpus(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	normalCases := loadModerationCorpusFile(t, "normal.tsv")
	violationCases := loadModerationCorpusFile(t, "violation.tsv")
	generatedNormalCases, generatedViolationCases := generatedModerationCorpusCases()
	cases := append(normalCases, generatedNormalCases...)
	cases = append(cases, violationCases...)
	cases = append(cases, generatedViolationCases...)
	results, metrics := evaluateModerationCorpus(t, moderator, cases, true)
	writeModerationRegressionReport(t, results, metrics)

	if metrics.FalsePositiveRate > maxFalsePositiveRate {
		t.Fatalf("false positive rate %.2f%% exceeds %.2f%% (%d/%d)",
			metrics.FalsePositiveRate*100, float64(maxFalsePositiveRate)*100,
			metrics.FalsePositive, metrics.NormalTotal)
	}
	if metrics.FalseNegativeRate > maxFalseNegativeRate {
		t.Fatalf("false negative rate %.2f%% exceeds %.2f%% (%d/%d)",
			metrics.FalseNegativeRate*100, float64(maxFalseNegativeRate)*100,
			metrics.FalseNegative, metrics.ViolationTotal)
	}
	if metrics.StrictMismatch > 0 {
		t.Fatalf("strict moderation corpus mismatches: %d; see %s",
			metrics.StrictMismatch, filepath.Join(moderationRegressionDir, moderationRegressionReportFile))
	}
}

func TestCommentModerationGrayCorpusReport(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)
	cases := loadModerationCorpusFile(t, "gray.tsv")
	results, metrics := evaluateModerationCorpus(t, moderator, cases, false)
	writeModerationGrayReport(t, results, metrics)
}

func loadCommentModerationRegressionConfig(t *testing.T) *appconfig.Config {
	t.Helper()

	root := findProjectRoot(t)
	reader := viper.New()
	reader.SetConfigType("yaml")
	reader.SetConfigFile(filepath.Join(root, "config/comment_moderation.yml"))
	if err := reader.ReadInConfig(); err != nil {
		t.Fatalf("read comment moderation config: %v", err)
	}

	var cfg appconfig.Config
	if err := reader.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal comment moderation config: %v", err)
	}
	if cfg.CommentModerationConfig == nil {
		t.Fatal("comment moderation config is nil")
	}
	return &cfg
}

func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("project root not found")
		}
		dir = parent
	}
}

func loadModerationCorpusFile(t *testing.T, name string) []moderationCorpusCase {
	t.Helper()

	path := filepath.Join(moderationRegressionDir, name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open moderation corpus %s: %v", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = false
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read moderation corpus %s: %v", path, err)
	}
	if len(records) == 0 {
		t.Fatalf("empty moderation corpus %s", path)
	}

	cases := make([]moderationCorpusCase, 0, len(records)-1)
	for line, record := range records {
		if len(record) == 0 || strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}
		if line == 0 {
			if strings.Join(record, "\t") != "id\ttext\texpected\tcategory\ttags\tnote" {
				t.Fatalf("unexpected header in %s: %v", path, record)
			}
			continue
		}
		if len(record) != 6 {
			t.Fatalf("invalid record in %s line %d: expected 6 fields, got %d", path, line+1, len(record))
		}
		item := moderationCorpusCase{
			ID:       strings.TrimSpace(record[0]),
			Text:     strings.TrimSpace(record[1]),
			Expected: strings.TrimSpace(record[2]),
			Category: strings.TrimSpace(record[3]),
			Tags:     strings.TrimSpace(record[4]),
			Note:     strings.TrimSpace(record[5]),
			Corpus:   strings.TrimSuffix(name, filepath.Ext(name)),
		}
		if item.ID == "" || item.Text == "" || item.Expected == "" {
			t.Fatalf("invalid empty id/text/expected in %s line %d", path, line+1)
		}
		cases = append(cases, item)
	}
	if len(cases) == 0 && name != "candidates.tsv" {
		t.Fatalf("no cases in moderation corpus %s", path)
	}
	return cases
}

func evaluateModerationCorpus(t *testing.T, moderator *moderation.Moderator,
	cases []moderationCorpusCase, strict bool) ([]moderationCorpusResult, moderationCorpusMetrics) {
	t.Helper()

	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.Local)
	results := make([]moderationCorpusResult, 0, len(cases))
	metrics := moderationCorpusMetrics{
		Total:              len(cases),
		CategoryWrongCount: make(map[string]int),
	}

	for index, item := range cases {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{
			UserID:    10001,
			ArticleID: 20002,
			ClientIP:  "127.0.0.1",
			Content:   item.Text,
			Now:       now.Add(time.Duration(index) * time.Second),
		}, nil)

		switch result.Status {
		case commentModel.StatusApproved:
			metrics.Approved++
		case commentModel.StatusRejected:
			metrics.Rejected++
		default:
			metrics.Pending++
		}

		wrong, falsePositive, falseNegative := classifyModerationCorpusResult(item, result.Status)
		switch item.Corpus {
		case "normal", "normal_generated":
			metrics.NormalTotal++
			if falsePositive {
				metrics.FalsePositive++
			}
		case "violation", "violation_generated":
			metrics.ViolationTotal++
			if falseNegative {
				metrics.FalseNegative++
			}
		case "gray":
			metrics.GrayTotal++
		}
		if wrong {
			if strict {
				metrics.StrictMismatch++
			} else {
				metrics.GrayMismatch++
			}
			metrics.CategoryWrongCount[item.Category]++
		}

		results = append(results, moderationCorpusResult{
			Case:   item,
			Result: result,
			Wrong:  wrong,
			Strict: strict,
		})
	}

	if metrics.NormalTotal > 0 {
		metrics.FalsePositiveRate = float64(metrics.FalsePositive) / float64(metrics.NormalTotal)
	}
	if metrics.ViolationTotal > 0 {
		metrics.FalseNegativeRate = float64(metrics.FalseNegative) / float64(metrics.ViolationTotal)
	}
	return results, metrics
}

func generatedModerationCorpusCases() ([]moderationCorpusCase, []moderationCorpusCase) {
	normalTemplates := []string{
		"这篇文章第 %d 段解释得很清楚，示例也比较完整。",
		"关于第 %d 个问题，我觉得还可以补充一点边界条件。",
		"这个方案在本地测试第 %d 次时表现稳定，感谢分享。",
		"第 %d 个代码片段里的变量命名可以再统一一些。",
		"我在 Go 里复现了第 %d 个例子，结果和文章一致。",
		"这篇教程对第 %d 个步骤的说明比较适合新手。",
		"第 %d 条评论只是讨论微信和 QQ 的识别策略，不包含联系方式。",
		"如果第 %d 个样例里出现 av 字符串，也可能只是 JSON 测试值。",
		"第 %d 次回归测试说明，关键词不能脱离上下文判断。",
		"文章第 %d 节对垃圾回收机制的解释很实用。",
	}
	violationTemplates := []string{
		"vx：b i n g b i n g s t u d e n t %04d",
		"qq：b i n g b i n g s t u d e n t %04d",
		"微信：b i n g b i n g s t u d e n t %04d",
		"v：b i n g b i n g s t u d e n t %04d",
		"加微信：bingbingstudent%04d，资料直接发你",
		"qq：study helper %04d，课程设计和实验报告都能处理",
		"微信：essay helper %04d，论文降重和查重报告都接",
		"vx：game boost %04d，代练代打刷分刷排名都可以",
		"qq：hot rank %04d，直播间人气和评论互动都能做",
		"微信：private query %04d，手机号住址都可以查",
		"vx：loan fast %04d，快速办理贷款不看征信",
		"qq：account deal %04d，二手账号老号实名号都有",
		"微信：adult resource %04d，高清无码资源私发",
		"vx：bet group %04d，博彩账号和下注技巧都有",
		"qq：tool share %04d，网站后台权限和数据库账号有办法进去",
	}

	normalCases := make([]moderationCorpusCase, 0, 2500)
	for i := 0; i < 2500; i++ {
		normalCases = append(normalCases, moderationCorpusCase{
			ID:       fmt.Sprintf("GN%04d", i+1),
			Text:     fmt.Sprintf(normalTemplates[i%len(normalTemplates)], i+1),
			Expected: commentModel.StatusApproved,
			Category: "generated_normal",
			Tags:     "generated,normal",
			Note:     "deterministic generated normal regression case",
			Corpus:   "normal_generated",
		})
	}

	violationCases := make([]moderationCorpusCase, 0, 2500)
	for i := 0; i < 2500; i++ {
		violationCases = append(violationCases, moderationCorpusCase{
			ID:       fmt.Sprintf("GV%04d", i+1),
			Text:     fmt.Sprintf(violationTemplates[i%len(violationTemplates)], i+1),
			Expected: "risk",
			Category: "generated_violation",
			Tags:     "generated,violation",
			Note:     "deterministic generated violation regression case",
			Corpus:   "violation_generated",
		})
	}
	return normalCases, violationCases
}

func classifyModerationCorpusResult(item moderationCorpusCase, actual string) (wrong, falsePositive, falseNegative bool) {
	expected := strings.ToLower(strings.TrimSpace(item.Expected))
	switch expected {
	case commentModel.StatusApproved:
		if actual != commentModel.StatusApproved {
			return true, true, false
		}
	case "risk":
		if actual == commentModel.StatusApproved {
			return true, false, true
		}
	case commentModel.StatusPending, commentModel.StatusRejected:
		if actual != expected {
			if actual == commentModel.StatusApproved {
				return true, false, true
			}
			return true, false, false
		}
	default:
		return true, false, false
	}
	return false, false, false
}

func writeModerationRegressionReport(t *testing.T, results []moderationCorpusResult,
	metrics moderationCorpusMetrics) {
	t.Helper()
	writeModerationCorpusReport(t, filepath.Join(moderationRegressionDir, moderationRegressionReportFile), results, metrics)
}

func writeModerationGrayReport(t *testing.T, results []moderationCorpusResult,
	metrics moderationCorpusMetrics) {
	t.Helper()
	writeModerationCorpusReport(t, filepath.Join(moderationRegressionDir, "gray_report.txt"), results, metrics)
}

func writeModerationCorpusReport(t *testing.T, path string, results []moderationCorpusResult,
	metrics moderationCorpusMetrics) {
	t.Helper()

	var b strings.Builder
	fmt.Fprintf(&b, "TOTAL cases=%d approved=%d pending=%d rejected=%d\n",
		metrics.Total, metrics.Approved, metrics.Pending, metrics.Rejected)
	fmt.Fprintf(&b, "NORMAL total=%d false_positive=%d false_positive_rate=%.2f%%\n",
		metrics.NormalTotal, metrics.FalsePositive, metrics.FalsePositiveRate*100)
	fmt.Fprintf(&b, "VIOLATION total=%d false_negative=%d false_negative_rate=%.2f%%\n",
		metrics.ViolationTotal, metrics.FalseNegative, metrics.FalseNegativeRate*100)
	if metrics.GrayTotal > 0 {
		fmt.Fprintf(&b, "GRAY total=%d mismatch=%d\n", metrics.GrayTotal, metrics.GrayMismatch)
	}
	writeModerationCategorySummary(&b, metrics.CategoryWrongCount)
	b.WriteString("\nCASES\n")
	generatedOKCount := 0
	const maxGeneratedOKReportRows = 40
	for _, result := range results {
		statusMarker := "ok"
		if result.Wrong {
			statusMarker = "wrong"
		}
		if isGeneratedModerationCorpus(result.Case.Corpus) && !result.Wrong {
			generatedOKCount++
			if generatedOKCount > maxGeneratedOKReportRows {
				continue
			}
		}
		fmt.Fprintf(&b, "%s | corpus=%s | expected=%s | actual=%s | risk_score=%d | category=%s | tags=%s | reasons=%s | text=%s\n",
			statusMarker,
			result.Case.Corpus,
			result.Case.Expected,
			result.Result.Status,
			result.Result.Score,
			result.Case.Category,
			result.Case.Tags,
			strings.Join(result.Result.Reasons, ";"),
			result.Case.Text,
		)
	}
	if generatedOKCount > maxGeneratedOKReportRows {
		fmt.Fprintf(&b, "ok | corpus=generated | omitted_ok_cases=%d | note=generated ok rows are truncated in report\n",
			generatedOKCount-maxGeneratedOKReportRows)
	}

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write moderation corpus report %s: %v", path, err)
	}
}

func isGeneratedModerationCorpus(corpus string) bool {
	return strings.HasSuffix(corpus, "_generated")
}

func writeModerationCategorySummary(b *strings.Builder, wrongCount map[string]int) {
	if len(wrongCount) == 0 {
		b.WriteString("WRONG_BY_CATEGORY none\n")
		return
	}
	b.WriteString("WRONG_BY_CATEGORY\n")
	categories := make([]string, 0, len(wrongCount))
	for category := range wrongCount {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	for _, category := range categories {
		fmt.Fprintf(b, "  %s=%d\n", category, wrongCount[category])
	}
}
