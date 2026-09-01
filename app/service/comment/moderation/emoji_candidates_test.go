package moderation

import (
	"context"
	"testing"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

func TestEmojiAnnotationIndexUsesUnicodeChineseData(t *testing.T) {
	index, err := resolveEmojiAnnotationIndex()
	if err != nil {
		t.Fatalf("resolveEmojiAnnotationIndex() error = %v", err)
	}
	if index.cldrVersion != "48.2.1" || index.emojiVersion != "16.0" || len(index.annotations) < 3000 {
		t.Fatalf("emoji annotation metadata = CLDR %q, Emoji %q, count %d",
			index.cldrVersion, index.emojiVersion, len(index.annotations))
	}
	occurrences := index.find("你🧠有🕳️吧")
	if len(occurrences) != 2 || !containsString(occurrences[0].Annotations, "脑") ||
		!containsString(occurrences[1].Annotations, "坑") {
		t.Fatalf("emoji occurrences = %+v, want brain/脑 and hole/坑 annotations", occurrences)
	}
	composite := index.find("👩‍💻")
	if len(composite) != 1 || composite[0].Text != "👩‍💻" {
		t.Fatalf("composite emoji = %+v, want one longest sequence", composite)
	}
}

func TestLocalContextAnalyzerMatchesEmojiConceptComposition(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localContextTestConfig()
	cfg.Lexicon.CustomWords.Review = map[string][]string{
		"abuse": {"脑子有坑"},
	}
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "你🧠有🕳️吧"},
		Text:    Normalize("你🧠有🕳️吧"),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	candidate, ok := findRewriteCandidate(assessment.Candidates, "🧠…🕳️", "脑子有坑")
	if !ok || candidate.Method != "emoji_annotation" || candidate.Category != "abuse" {
		t.Fatalf("emoji candidates = %+v, want 🧠…🕳️ -> 脑子有坑", assessment.Candidates)
	}
	decision := fuseEvidence(assessment.Evidence, assessment, cfg.DecisionEngine)
	if assessment.Intent != "targeted_abuse" || decision.Status != commentModel.StatusRejected {
		t.Fatalf("emoji assessment = %+v, decision = %+v", assessment, decision)
	}
}

func TestNormalizePreservesEmojiAndFullPinyin(t *testing.T) {
	emoji := Normalize("你🧠有🕳️吧")
	if emoji.Normalized != "你🧠有🕳吧" {
		t.Fatalf("Normalize() replaced emoji: %q", emoji.Normalized)
	}
	pinyin := Normalize("dijiachushou")
	if pinyin.Normalized != "dijiachushou" || pinyin.Compact != "dijiachushou" {
		t.Fatalf("Normalize() folded full pinyin: %+v", pinyin)
	}
}

func TestLocalContextAnalyzerIgnoresBenignEmojiText(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := appconfig.CommentModerationConfig{}
	ApplyDefaults(&cfg)
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "今天天气真好🌞"},
		Text:    Normalize("今天天气真好🌞"),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(assessment.Candidates) != 0 {
		t.Fatalf("benign emoji text produced risk candidates: %+v", assessment.Candidates)
	}
}

func TestDecisionEngineUsesEmojiCandidate(t *testing.T) {
	cfg := localContextTestConfig()
	cfg.Lexicon.CustomWords.Review = map[string][]string{
		"abuse": {"脑子有坑"},
	}
	moderator := NewModerator(staticModerationConfig{cfg: cfg}, zap.NewNop(), nil)
	result := moderator.ModerateWithBehavior(context.Background(), Request{Content: "你🧠有🕳️吧"}, nil)
	trace := result.Trace.DecisionEngine
	if trace == nil || trace.Decision.Status != commentModel.StatusRejected || !trace.Decision.Actionable {
		t.Fatalf("emoji decision trace = %+v", trace)
	}
	if !containsRewriteCandidate(trace.Candidates, "🧠…🕳️", "脑子有坑") {
		t.Fatalf("decision candidates = %+v, want emoji concept candidate", trace.Candidates)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
