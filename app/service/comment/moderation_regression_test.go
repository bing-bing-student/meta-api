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
	moderationRegressionDir        = "testdata"
	maxFalsePositiveRate           = 0
	maxFalseNegativeRate           = 0
	minimumStrictCorpusCases       = 3000
	moderationRegressionReportFile = "report.txt"
)

var (
	normalModerationCorpusFiles    = []string{"normal.tsv", "normal_blog_generated.tsv"}
	violationModerationCorpusFiles = []string{"violation.tsv", "violation_blog_generated.tsv"}
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
	ManualEdge         moderationManualEdgeMetrics
}

type moderationManualEdgeMetrics struct {
	Total          int
	NormalTotal    int
	ViolationTotal int
	Approved       int
	Pending        int
	Rejected       int
	FalsePositive  int
	FalseNegative  int
}

func TestCommentModerationGoldenCorpus(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	normalCases := loadModerationCorpusFiles(t, normalModerationCorpusFiles...)
	violationCases := loadModerationCorpusFiles(t, violationModerationCorpusFiles...)
	cases := append(normalCases, violationCases...)
	validateStrictModerationCorpus(t, cases)
	results, metrics := evaluateModerationCorpus(t, moderator, cases, true)
	writeModerationRegressionReport(t, results, metrics)
	assertModerationCorpusMetrics(t, metrics, moderationRegressionReportFile)
}

func loadModerationCorpusFiles(t *testing.T, names ...string) []moderationCorpusCase {
	t.Helper()
	result := make([]moderationCorpusCase, 0)
	for _, name := range names {
		result = append(result, loadModerationCorpusFile(t, name)...)
	}
	return result
}

func validateStrictModerationCorpus(t *testing.T, cases []moderationCorpusCase) {
	t.Helper()
	if len(cases) < minimumStrictCorpusCases {
		t.Fatalf("strict moderation corpus has %d cases, want at least %d", len(cases), minimumStrictCorpusCases)
	}
	ids := make(map[string]struct{}, len(cases))
	texts := make(map[string]string, len(cases))
	for _, item := range cases {
		if _, exists := ids[item.ID]; exists {
			t.Fatalf("duplicate moderation corpus id %q", item.ID)
		}
		ids[item.ID] = struct{}{}
		if previousID, exists := texts[item.Text]; exists {
			t.Fatalf("duplicate moderation corpus text in %s and %s: %q", previousID, item.ID, item.Text)
		}
		texts[item.Text] = item.ID
	}
}

func assertModerationCorpusMetrics(t *testing.T, metrics moderationCorpusMetrics, reportFile string) {
	t.Helper()
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
			metrics.StrictMismatch, filepath.Join(moderationRegressionDir, reportFile))
	}
}

func TestCommentModerationGrayCorpusReport(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)
	cases := loadModerationCorpusFile(t, "gray.tsv")
	results, metrics := evaluateModerationCorpus(t, moderator, cases, false)
	writeModerationGrayReport(t, results, metrics)
}

func TestCommentModerationPrivateChatVariantRegression(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	for _, content := range []string{
		"有资源的丝聊我",
		"有资源的司聊我",
		"有资源的撕聊我",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status == commentModel.StatusApproved {
			t.Errorf("Moderate(%q) = approved, want pending or rejected; trace = %+v", content, result.Trace)
		}
	}

	for _, content := range []string{
		"不要私聊我，请公开回复",
		"不要丝聊我，请公开回复",
		"“丝聊”是“私聊”的同音错别字吗？",
		"审核系统应识别‘有资源的丝聊我’，但不要误伤正常讨论。",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusApproved {
			t.Errorf("Moderate(%q) = %s, want approved; score = %d context = %+v evidence = %+v fusion = %+v",
				content, result.Status, result.Score, result.Trace.DecisionEngine.Context,
				result.Trace.DecisionEngine.Evidence, result.Trace.DecisionEngine.Fusion)
		}
	}
}

func TestCommentModerationExpandedRelationBoundaryRegression(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	for _, content := range []string{
		"这作者脑子是不是有🕳️，这都能写错",
		"无m资源还有，想看的私",
		"答辩稿我可以帮你弄完，时间紧也来得及。",
		"选题我可以直接帮你，交付以后再结算。",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status == commentModel.StatusApproved {
			t.Errorf("Moderate(%q) = approved, want risk; trace = %+v", content, result.Trace)
		}
	}

	for _, content := range []string{
		"这篇关于数据库连接池的文章异常处理部分写得很细致，我准备在本地项目里验证一下。",
		"这篇文章的示例代码容易跟着复现，我准备在本地项目里验证一下。",
		"如果有人发布“赌博平台”，管理员应该怎样处理？",
		"如果有人发布‘加群领取资料’，管理员应该怎样处理？",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusApproved {
			t.Errorf("Moderate(%q) = %s, want approved; score = %d trace = %+v",
				content, result.Status, result.Score, result.Trace)
		}
	}
}

func TestCommentModerationHarmfulValueRelationRegression(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	for _, content := range []string{
		"你怎么不去跳楼",
		"你去喝敌敌畏吧",
		"他有本事就去割腕",
		"你怎么不去跳搂",
		"你去喝敌敌威吧",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusRejected {
			t.Errorf("Moderate(%q) = %s, want rejected; score = %d trace = %+v",
				content, result.Status, result.Score, result.Trace)
		}
	}

	for _, content := range []string{
		"今天上课老师科普法轮功的知识，让我们了解其危害",
		"今天上课老师科普赌博平台的知识，让我们了解其危害",
		"今天上课老师科普未成年人私照的知识，让我们了解其危害",
		"我劝你不要跳楼，有问题及时求助",
		"警方救下准备跳楼的人",
		"敌敌畏有毒，误服后应立即就医",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusApproved {
			engine := result.Trace.DecisionEngine
			t.Errorf("Moderate(%q) = %s, want approved; score = %d context = %+v evidence = %+v fusion = %+v",
				content, result.Status, result.Score, engine.Context, engine.Evidence, engine.Fusion)
		}
	}

	for _, content := range []string{
		"我最近压力很大，开始想跳楼",
		"我感觉自己活不下去，打算割腕",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusPending {
			t.Errorf("Moderate(%q) = %s, want pending for human care; score = %d trace = %+v",
				content, result.Status, result.Score, result.Trace)
		}
	}

	result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{
		Content: "老师让我们加入法轮功",
	}, nil)
	if result.Status == commentModel.StatusApproved {
		t.Errorf("promotional education wrapper was approved; trace = %+v", result.Trace)
	}
}

func TestCommentModerationContentCriticismBoundaryRegression(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)
	for _, content := range []string{
		"这教程写得跟U0001f4a9一样，真的服",
		"懂个U0001f528还在这指导别人",
		"ni zhe shui ping 也敢出来教别人？",
		"bu dong zhuang dong，还在这里误导新人",
		"pian liu liang 也不用这么明显吧",
		"wu ren zi di 的教程就别继续发了",
		"zhen e xin，这种内容看着就烦",
		"dong ge chui zi 还出来指点别人",
		"di ji de cuo wu 都能犯，真服",
		"别出来 wu dao xin ren 了，容易把人带沟里",
		"zhuan men chu lai wu dao xin ren 是吧",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusApproved {
			engine := result.Trace.DecisionEngine
			t.Errorf("Moderate(%q) = %s, want approved; score = %d context = %+v evidence = %+v fusion = %+v",
				content, result.Status, result.Score, engine.Context, engine.Evidence, engine.Fusion)
		}
	}
}

func TestCommentModerationStanceEvaluationRegression(t *testing.T) {
	cfg := loadCommentModerationRegressionConfig(t)
	moderator := moderation.NewModerator(cfg, zap.NewNop(), nil)

	for _, content := range []string{
		"拒绝毕业设计代做从我做起。",
		"毕业设计代做属于非法产业。",
		"论文代写本质上是违法行为。",
		"毕业设计代做应当严厉打击。",
		"评论区私聊领取资源基本都是诈骗。",
		"私聊发资源这种套路不可信，建议直接举报。",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status != commentModel.StatusApproved {
			engine := result.Trace.DecisionEngine
			t.Errorf("Moderate(%q) = %s, want approved; score = %d context = %+v evidence = %+v fusion = %+v",
				content, result.Status, result.Score, engine.Context, engine.Evidence, engine.Fusion)
		}
	}

	for _, content := range []string{
		"毕业设计我可以直接帮你，交付以后再结算。",
		"毕业设计代做属于非法产业不过我仍然接单。",
		"毕业设计代做属于非法产业，但我可以接单。",
		"毕业设计代做不属于非法产业，我可以接单。",
	} {
		result := moderator.ModerateWithBehavior(context.Background(), moderation.Request{Content: content}, nil)
		if result.Status == commentModel.StatusApproved {
			t.Errorf("Moderate(%q) = approved, want pending or rejected; trace = %+v", content, result.Trace)
		}
	}
}

func loadCommentModerationRegressionConfig(t *testing.T) *appconfig.Config {
	t.Helper()

	root := findProjectRoot(t)
	reader := viper.New()
	reader.SetConfigType("yaml")
	files := []string{
		"config/comment_moderation.manifest.yml",
		"config/moderation/categories.yml",
		"config/moderation/concepts.yml",
		"config/moderation/lexicon.yml",
		"config/moderation/packs/safety.yml",
		"config/moderation/packs/commerce.yml",
		"config/moderation/packs/privacy_security.yml",
		"config/moderation/packs/contextual_safety.yml",
		"config/moderation/semantics.yml",
		"config/moderation/behavior.yml",
		"config/moderation/calibration.yml",
	}
	for index, name := range files {
		reader.SetConfigFile(filepath.Join(root, name))
		var err error
		if index == 0 {
			err = reader.ReadInConfig()
		} else {
			err = reader.MergeInConfig()
		}
		if err != nil {
			t.Fatalf("read comment moderation policy %s: %v", name, err)
		}
	}

	var cfg appconfig.Config
	if err := reader.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal comment moderation config: %v", err)
	}
	if cfg.CommentModerationConfig == nil {
		t.Fatal("comment moderation config is nil")
	}
	// Viper 会覆盖同名数组，而生产加载器会按策略包顺序追加数组；测试中显式重建组合规则，
	// 保证黄金集使用的策略与生产环境完全一致。
	cfg.CommentModerationConfig.CombinationRules = nil
	for _, name := range []string{
		"config/moderation/packs/safety.yml",
		"config/moderation/packs/commerce.yml",
		"config/moderation/packs/privacy_security.yml",
		"config/moderation/packs/contextual_safety.yml",
	} {
		packReader := viper.New()
		packReader.SetConfigType("yaml")
		packReader.SetConfigFile(filepath.Join(root, name))
		if err := packReader.ReadInConfig(); err != nil {
			t.Fatalf("read moderation rule pack %s: %v", name, err)
		}
		var fragment appconfig.Config
		if err := packReader.Unmarshal(&fragment); err != nil || fragment.CommentModerationConfig == nil {
			t.Fatalf("unmarshal moderation rule pack %s: %v", name, err)
		}
		cfg.CommentModerationConfig.CombinationRules = append(
			cfg.CommentModerationConfig.CombinationRules,
			fragment.CommentModerationConfig.CombinationRules...,
		)
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
			Corpus:   moderationCorpusKind(name),
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

func moderationCorpusKind(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if index := strings.IndexByte(base, '_'); index >= 0 {
		base = base[:index]
	}
	return base
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
		manualEdge := hasModerationCorpusTag(item.Tags, "manual_edge")

		switch result.Status {
		case commentModel.StatusApproved:
			metrics.Approved++
			if manualEdge {
				metrics.ManualEdge.Approved++
			}
		case commentModel.StatusRejected:
			metrics.Rejected++
			if manualEdge {
				metrics.ManualEdge.Rejected++
			}
		default:
			metrics.Pending++
			if manualEdge {
				metrics.ManualEdge.Pending++
			}
		}

		wrong, falsePositive, falseNegative := classifyModerationCorpusResult(item, result.Status)
		if manualEdge {
			metrics.ManualEdge.Total++
			switch item.Corpus {
			case "normal":
				metrics.ManualEdge.NormalTotal++
				if falsePositive {
					metrics.ManualEdge.FalsePositive++
				}
			case "violation":
				metrics.ManualEdge.ViolationTotal++
				if falseNegative {
					metrics.ManualEdge.FalseNegative++
				}
			}
		}
		switch item.Corpus {
		case "normal":
			metrics.NormalTotal++
			if falsePositive {
				metrics.FalsePositive++
			}
		case "violation":
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

func hasModerationCorpusTag(tags, want string) bool {
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(tag) == want {
			return true
		}
	}
	return false
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
	if metrics.ManualEdge.Total > 0 {
		manualFalsePositiveRate := float64(metrics.ManualEdge.FalsePositive) /
			float64(metrics.ManualEdge.NormalTotal)
		manualFalseNegativeRate := float64(metrics.ManualEdge.FalseNegative) /
			float64(metrics.ManualEdge.ViolationTotal)
		fmt.Fprintf(&b,
			"MANUAL_EDGE total=%d approved=%d pending=%d rejected=%d normal=%d false_positive=%d false_positive_rate=%.2f%% violation=%d false_negative=%d false_negative_rate=%.2f%%\n",
			metrics.ManualEdge.Total,
			metrics.ManualEdge.Approved,
			metrics.ManualEdge.Pending,
			metrics.ManualEdge.Rejected,
			metrics.ManualEdge.NormalTotal,
			metrics.ManualEdge.FalsePositive,
			manualFalsePositiveRate*100,
			metrics.ManualEdge.ViolationTotal,
			metrics.ManualEdge.FalseNegative,
			manualFalseNegativeRate*100,
		)
	}
	if metrics.GrayTotal > 0 {
		fmt.Fprintf(&b, "GRAY total=%d mismatch=%d\n", metrics.GrayTotal, metrics.GrayMismatch)
	}
	writeModerationCategorySummary(&b, metrics.CategoryWrongCount)
	b.WriteString("\nCASES\n")
	for _, result := range results {
		statusMarker := "ok"
		if result.Wrong {
			statusMarker = "wrong"
		}
		fmt.Fprintf(&b, "%s | id=%s | corpus=%s | expected=%s | actual=%s | risk_score=%d | category=%s | tags=%s | reasons=%s | text=%s\n",
			statusMarker,
			result.Case.ID,
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

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write moderation corpus report %s: %v", path, err)
	}
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
