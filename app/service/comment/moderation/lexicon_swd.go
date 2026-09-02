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

// LexiconDetector 定义敏感词检测器契约。
// Detect 的输入为调用上下文、归一化评论和审核配置；返回命中信号及检测错误。
type LexiconDetector interface {
	Detect(ctx context.Context, text NormalizedComment, cfg appconfig.CommentModerationConfig) ([]Signal, error)
}

// swdLexiconDetector 封装 go-swd 引擎、配置签名和自定义词等级，并支持并发读取及配置热更新。
type swdLexiconDetector struct {
	logger *zap.Logger

	mu              sync.RWMutex
	engine          *swd.SWD
	configSignature string
	customLevels    map[string]string
}

// NewSWDLexiconDetector 创建并初始化 SWD 词典检测器。
// 输入 logger 用于记录运行信息；返回检测器接口，基础词典初始化失败时返回错误。
func NewSWDLexiconDetector(logger *zap.Logger) (LexiconDetector, error) {
	detector := &swdLexiconDetector{logger: logger}
	if err := detector.reload(appconfig.CommentModerationConfig{}); err != nil {
		return nil, err
	}
	return detector, nil
}

// Detect 在评论的多个文本视图中执行词典匹配，并将结果转换为去重审核信号。
// 输入 ctx 控制取消，text 提供文本视图，cfg 提供词典和等级配置；返回信号集合或加载、取消错误。
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
				Score:    evidenceStrengthScore(SourceLexicon, category, "", level, cfg),
				Reason:   formatReason(SourceLexicon, category, level, match.Word),
				Evidence: match.Word,
			})
		}
	}
	return signals, nil
}

// shouldSkipSWDMatch 判断 match 是否位于技术词、JSON 值或较长 ASCII 单词内部。
// 输入 view 是当前文本视图；返回 true 表示该词典命中应作为边界误报跳过。
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

// isQuotedJSONValueMatch 判断 match 是否恰好是 JSON 键值结构中的带引号值。
// 输入 view 是文本、match 含字符下标；返回 true 表示可按技术上下文过滤。
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

// isBenignChineseTechnicalTermMatch 过滤具有明确技术含义的中文词典命中。
// 输入 view、word 和 match 分别为全文、命中词及位置；返回 true 表示属于已识别的良性技术搭配。
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

// isShortASCIIWord 判断 value 是否为不超过两个字符的 ASCII 单词；返回对应结果。
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

// isASCIIWordRune 判断 r 是否为 ASCII 字母、数字或下划线；返回对应结果。
func isASCIIWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// ensureConfig 确保检测器已加载与 cfg 签名一致的 SWD 引擎配置。
// 输入 cfg 是当前审核配置；配置未变化时直接返回 nil，重载失败时返回错误。
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

// reload 在持有写锁的情况下根据 cfg 重建检测器状态。
// 返回初始化或自定义词加载错误；适用于外部尚未持锁的调用方。
func (d *swdLexiconDetector) reload(cfg appconfig.CommentModerationConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reloadLocked(cfg, lexiconConfigSignature(cfg))
}

// reloadLocked 根据 cfg 和预计算 signature 重建 SWD 引擎并替换缓存状态。
// 调用方必须已持有写锁；返回引擎创建、清理或加词错误。
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

// levelForMatch 解析命中 word 的审核等级，优先使用自定义词等级，再使用分类默认等级。
// 输入 category 和 cfg 用于兜底；返回规范化等级，关键分类缺失配置时采用最低安全兜底。
func (d *swdLexiconDetector) levelForMatch(word, category string, customLevels map[string]string,
	cfg appconfig.CommentModerationConfig) string {
	if level := normalizeLevel(customLevels[word]); level != "" {
		return level
	}
	if registered, ok := cfg.Categories[category]; ok {
		if level := normalizeLevel(registered.DefaultLevel); level != "" {
			return level
		}
	}
	// 保留不可配置的最低安全兜底；正常启动时分类等级来自 categories.yml。
	switch category {
	case "sexual", "gambling", "drugs":
		return LevelBlock
	default:
		return LevelReview
	}
}

// buildSWDCustomWords 将配置中的拒绝词和待审核词转换为 SWD 分类映射及等级映射。
// 返回值依次供词典引擎加载和命中等级解析使用。
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

// swdCategoryName 将 go-swd 分类枚举 value 转换为审核系统分类标识并返回。
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

// inferSensitiveCategory 根据未细分类词项 word 的内容推断审核分类。
// 返回赌博、毒品、涉政、色情、暴力、诈骗或通用 sensitive 分类。
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

// swdCategoryFromName 将配置中的中英文分类名称 value 转换为 go-swd 分类枚举并返回。
// 无法识别时返回自定义分类。
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

// lexiconConfigSignature 为影响 SWD 引擎内容的 cfg 字段生成稳定签名并返回。
// 该签名用于判断是否需要热重载词典。
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

// wordMapSignature 将指定 prefix 下的分类词表 values 转换为稳定排序的签名片段并返回。
func wordMapSignature(prefix string, values map[string][]string) []string {
	parts := make([]string, 0, len(values))
	for category, words := range values {
		items := append([]string{}, words...)
		sort.Strings(items)
		parts = append(parts, prefix+":"+category+"="+strings.Join(items, ","))
	}
	return parts
}

// cloneStringMap 复制 src，返回可脱离锁安全读取的新映射；空输入返回 nil。
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
