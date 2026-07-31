package moderation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/kirklin/go-swd"
	swdcategory "github.com/kirklin/go-swd/pkg/types/category"
	"go.uber.org/zap"

	appconfig "meta-api/config"
)

type LexiconDetector interface {
	Detect(ctx context.Context, text NormalizedComment, cfg appconfig.CommentModerationConfig) ([]Signal, error)
}

type swdLexiconDetector struct {
	logger *zap.Logger

	mu              sync.RWMutex
	engine          *swd.SWD
	configSignature string
	customLevels    map[string]string
}

func NewSWDLexiconDetector(logger *zap.Logger) (LexiconDetector, error) {
	detector := &swdLexiconDetector{logger: logger}
	if err := detector.reload(appconfig.CommentModerationConfig{}); err != nil {
		return nil, err
	}
	return detector, nil
}

func (d *swdLexiconDetector) Detect(ctx context.Context, text NormalizedComment,
	cfg appconfig.CommentModerationConfig) ([]Signal, error) {
	if err := d.ensureConfig(cfg); err != nil {
		return nil, err
	}

	d.mu.RLock()
	engine := d.engine
	customLevels := cloneStringMap(d.customLevels)
	d.mu.RUnlock()
	if engine == nil {
		return nil, nil
	}

	signals := make([]Signal, 0)
	seen := make(map[string]struct{})
	for _, view := range text.Views() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		view = strings.TrimSpace(view)
		if view == "" {
			continue
		}
		for _, match := range engine.MatchAll(view) {
			if shouldSkipSWDMatch(view, match) {
				continue
			}
			category := swdCategoryName(match.Category)
			if category == "sensitive" {
				category = inferSensitiveCategory(match.Word)
			}
			level := d.levelForMatch(match.Word, category, customLevels, cfg)
			key := category + ":" + level + ":" + match.Word
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			signals = append(signals, Signal{
				Source:   SourceLexicon,
				Category: category,
				Level:    level,
				Score:    scoreForSignal(SourceLexicon, category, "", level, cfg),
				Reason:   formatReason(SourceLexicon, category, level, match.Word),
				Evidence: match.Word,
			})
		}
	}
	return signals, nil
}

func shouldSkipSWDMatch(view string, match swd.SensitiveWord) bool {
	word := strings.ToLower(strings.TrimSpace(match.Word))
	if word == "" {
		return false
	}
	if isBenignChineseTechnicalTermMatch(view, word, match) {
		return true
	}
	if !isShortASCIIWord(word) {
		return false
	}
	if isQuotedJSONValueMatch(view, match) {
		return true
	}

	runes := []rune(view)
	if match.StartPos < 0 || match.EndPos > len(runes) || match.StartPos >= match.EndPos {
		return false
	}
	if match.StartPos > 0 && isASCIIWordRune(runes[match.StartPos-1]) {
		return true
	}
	if match.EndPos < len(runes) && isASCIIWordRune(runes[match.EndPos]) {
		return true
	}
	return false
}

func isQuotedJSONValueMatch(view string, match swd.SensitiveWord) bool {
	runes := []rune(view)
	if match.StartPos <= 0 || match.EndPos >= len(runes) || match.StartPos >= match.EndPos {
		return false
	}
	if runes[match.StartPos-1] != '"' || runes[match.EndPos] != '"' {
		return false
	}
	i := match.StartPos - 2
	for i >= 0 && strings.TrimSpace(string(runes[i])) == "" {
		i--
	}
	return i >= 0 && (runes[i] == ':' || runes[i] == '：')
}

func isBenignChineseTechnicalTermMatch(view, word string, match swd.SensitiveWord) bool {
	if word != "垃圾" {
		return false
	}

	runes := []rune(view)
	if match.StartPos < 0 || match.EndPos > len(runes) || match.StartPos >= match.EndPos {
		return false
	}
	after := string(runes[match.EndPos:])
	return strings.HasPrefix(after, "回收") || strings.HasPrefix(after, "收集")
}

func isShortASCIIWord(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 || len(runes) > 2 {
		return false
	}
	for _, r := range runes {
		if !isASCIIWordRune(r) {
			return false
		}
	}
	return true
}

func isASCIIWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func (d *swdLexiconDetector) ensureConfig(cfg appconfig.CommentModerationConfig) error {
	signature := lexiconConfigSignature(cfg)

	d.mu.RLock()
	same := d.engine != nil && d.configSignature == signature
	d.mu.RUnlock()
	if same {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.engine != nil && d.configSignature == signature {
		return nil
	}
	return d.reloadLocked(cfg, signature)
}

func (d *swdLexiconDetector) reload(cfg appconfig.CommentModerationConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reloadLocked(cfg, lexiconConfigSignature(cfg))
}

func (d *swdLexiconDetector) reloadLocked(cfg appconfig.CommentModerationConfig, signature string) error {
	engine, err := swd.New()
	if err != nil {
		return fmt.Errorf("create go-swd detector: %w", err)
	}
	if !cfg.Lexicon.UseBuiltin && strings.TrimSpace(cfg.Lexicon.Provider) != "" {
		if err = engine.Clear(); err != nil {
			return fmt.Errorf("clear go-swd builtin words: %w", err)
		}
	}

	customWords, customLevels := buildSWDCustomWords(cfg.Lexicon.CustomWords)
	if len(customWords) > 0 {
		if err = engine.AddWords(customWords); err != nil {
			return fmt.Errorf("add go-swd custom words: %w", err)
		}
	}

	d.engine = engine
	d.configSignature = signature
	d.customLevels = customLevels
	return nil
}

func (d *swdLexiconDetector) levelForMatch(word, category string, customLevels map[string]string,
	cfg appconfig.CommentModerationConfig) string {
	if level := normalizeLevel(customLevels[word]); level != "" {
		return level
	}
	if override, ok := cfg.Decision.CategoryOverrides[category]; ok {
		if level := normalizeLevel(override.Level); level != "" {
			return level
		}
	}
	switch category {
	case "sexual", "gambling", "drugs":
		return LevelBlock
	default:
		return LevelReview
	}
}

func buildSWDCustomWords(cfg appconfig.CommentModerationCustomWordsConfig) (map[string]swd.Category, map[string]string) {
	words := make(map[string]swd.Category)
	levels := make(map[string]string)
	appendWords := func(level string, values map[string][]string) {
		for categoryName, items := range values {
			category := swdCategoryFromName(categoryName)
			for _, item := range items {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				words[item] = category
				levels[item] = level
			}
		}
	}
	appendWords(LevelBlock, cfg.Block)
	appendWords(LevelReview, cfg.Review)
	return words, levels
}

func swdCategoryName(value swdcategory.Category) string {
	switch value {
	case swd.Pornography:
		return "sexual"
	case swd.Political:
		return "political"
	case swd.Violence:
		return "violence"
	case swd.Gambling:
		return "gambling"
	case swd.Drugs:
		return "drugs"
	case swd.Profanity:
		return "abuse"
	case swd.Discrimination:
		return "hate"
	case swd.Scam:
		return "spam_fraud"
	case swd.Custom:
		return "custom"
	default:
		return "sensitive"
	}
}

func inferSensitiveCategory(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	switch {
	case strings.Contains(word, "赌") || strings.Contains(word, "博彩") || strings.Contains(word, "下注") ||
		strings.Contains(word, "娱乐城"):
		return "gambling"
	case strings.Contains(word, "毒品") || strings.Contains(word, "制毒") || strings.Contains(word, "贩毒") ||
		strings.Contains(word, "吸毒"):
		return "drugs"
	case strings.Contains(word, "法轮功") || strings.Contains(word, "涉政") || strings.Contains(word, "政权") ||
		strings.Contains(word, "推翻政府"):
		return "political"
	case strings.Contains(word, "色情") || strings.Contains(word, "成人") || strings.Contains(word, "裸聊") ||
		strings.Contains(word, "约炮") || strings.Contains(word, "淫"):
		return "sexual"
	case strings.Contains(word, "暴力") || strings.Contains(word, "杀人") || strings.Contains(word, "砍人") ||
		strings.Contains(word, "打人"):
		return "violence"
	case strings.Contains(word, "诈骗") || strings.Contains(word, "刷单") || strings.Contains(word, "贷款"):
		return "spam_fraud"
	default:
		return "sensitive"
	}
}

func swdCategoryFromName(value string) swd.Category {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sexual", "pornography", "涉黄":
		return swd.Pornography
	case "political", "涉政":
		return swd.Political
	case "violence", "暴力":
		return swd.Violence
	case "gambling", "赌博":
		return swd.Gambling
	case "drugs", "毒品":
		return swd.Drugs
	case "abuse", "profanity", "脏话":
		return swd.Profanity
	case "hate", "discrimination", "歧视":
		return swd.Discrimination
	case "spam_fraud", "scam", "诈骗":
		return swd.Scam
	default:
		return swd.Custom
	}
}

func lexiconConfigSignature(cfg appconfig.CommentModerationConfig) string {
	parts := []string{
		"provider=" + strings.TrimSpace(cfg.Lexicon.Provider),
		fmt.Sprintf("builtin=%t", cfg.Lexicon.UseBuiltin),
		fmt.Sprintf("strict=%t", cfg.Lexicon.StrictBuiltinMatch),
	}
	parts = append(parts, wordMapSignature("block", cfg.Lexicon.CustomWords.Block)...)
	parts = append(parts, wordMapSignature("review", cfg.Lexicon.CustomWords.Review)...)
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func wordMapSignature(prefix string, values map[string][]string) []string {
	parts := make([]string, 0, len(values))
	for category, words := range values {
		items := append([]string{}, words...)
		sort.Strings(items)
		parts = append(parts, prefix+":"+category+"="+strings.Join(items, ","))
	}
	return parts
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
