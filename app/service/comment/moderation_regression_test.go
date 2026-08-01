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

	normalCases := loadModerationCorpusFile(t, "normal.tsv")
	violationCases := loadModerationCorpusFile(t, "violation.tsv")
	cases := append(normalCases, violationCases...)
	results, metrics := evaluateModerationCorpus(t, moderator, cases, true)
	writeModerationRegressionReport(t, results, metrics)
	assertModerationCorpusMetrics(t, metrics, moderationRegressionReportFile)
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
