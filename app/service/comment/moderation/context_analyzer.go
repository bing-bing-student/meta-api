package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/kirklin/go-swd/pkg/dictionary"
	swdcategory "github.com/kirklin/go-swd/pkg/types/category"
	"github.com/mozillazg/go-pinyin"
	"go.uber.org/zap"

	appconfig "meta-api/config"
)

var (
	localASCIIWordRegexp = regexp.MustCompile(`[a-z]{2,20}`)
	localHanRunRegexp    = regexp.MustCompile(`\p{Han}{2,}`)
)

type ContextInput struct {
	Request    Request
	Text       NormalizedComment
	Candidates []RewriteCandidate
	Evidence   []Evidence
}

type ContextAnalyzer interface {
	Analyze(context.Context, ContextInput, appconfig.CommentModerationConfig) (ContextAssessment, error)
}

type riskTerm struct {
	Text            string
	Category        string
	Role            string
	Compact         string
	Pinyin          string
	Initials        string
	RuneCount       int
	PinyinPattern   *regexp.Regexp
	InitialsPattern *regexp.Regexp
	AllowPureASCII  bool
}

type riskTermIndex struct {
	byPinyin      map[string][]riskTerm
	byASCIIPinyin map[string][]riskTerm
	byInitials    map[string][]riskTerm
	byLength      map[int][]riskTerm
	lengths       []int
	terms         []riskTerm
}

type LocalContextAnalyzer struct {
	logger   *zap.Logger
	builtins []riskTerm

	mu        sync.RWMutex
	signature string
	index     riskTermIndex
}

func NewLocalContextAnalyzer(logger *zap.Logger) *LocalContextAnalyzer {
	analyzer := &LocalContextAnalyzer{logger: logger}
	builtins, err := loadBuiltinRiskTerms()
	if err != nil {
		if logger != nil {
			logger.Warn("load builtin risk terms for local context analysis failed", zap.Error(err))
		}
		return analyzer
	}
	analyzer.builtins = builtins
	return analyzer
}

func (a *LocalContextAnalyzer) Analyze(ctx context.Context, input ContextInput,
	cfg appconfig.CommentModerationConfig,
) (ContextAssessment, error) {
	if cfg.DecisionEngine.ContextAnalysis.Disabled {
		return ContextAssessment{UnavailableReason: "context_analysis_disabled"}, nil
	}
	if err := ctx.Err(); err != nil {
		return ContextAssessment{}, err
	}
	index, err := a.resolveIndex(cfg)
	if err != nil {
		return ContextAssessment{}, err
	}
	maxCandidates := cfg.DecisionEngine.ContextAnalysis.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 16
	}
	variantCandidates, err := localVariantCandidates(ctx, input.Text, index, maxCandidates)
	if err != nil {
		return ContextAssessment{}, err
	}
	applySurroundingContext(variantCandidates, input.Request)
	relations := analyzeSemanticRelations(input.Text, variantCandidates, input.Evidence, cfg)
	profile := relationIntentProfile(relations, input.Text, cfg)
	localEvidence := append(candidateEvidence(variantCandidates, relations, cfg), relationEvidence(relations)...)

	probabilities := make(map[string]float64)
	for _, item := range append(append([]Evidence(nil), input.Evidence...), localEvidence...) {
		if item.Polarity != "positive" || item.Category == "" || item.Category == "benign_context" {
			continue
		}
		if item.Confidence > probabilities[item.Category] {
			probabilities[item.Category] = item.Confidence
		}
	}
	for category, probability := range probabilities {
		if profile.boost > 0 {
			probability += profile.boost
		}
		probabilities[category] = clampProbability(probability)
	}

	return ContextAssessment{
		Analyzed:              true,
		Confidence:            profile.confidence,
		Intent:                profile.intent,
		BenignProbability:     profile.benignProbability,
		CategoryProbabilities: probabilities,
		Candidates:            variantCandidates,
		Evidence:              localEvidence,
		Relations:             relations,
		Explanation:           profile.explanation,
	}, nil
}

func (a *LocalContextAnalyzer) resolveIndex(cfg appconfig.CommentModerationConfig) (riskTermIndex, error) {
	encoded, err := json.Marshal(struct {
		Context       appconfig.CommentModerationContextAnalysisConfig
		Custom        appconfig.CommentModerationCustomWordsConfig
		Fuzzy         appconfig.CommentModerationFuzzyConfig
		Combo         []appconfig.CommentModerationCombinationRuleConfig
		HarmfulPolicy appconfig.CommentModerationHarmfulValuePolicyConfig
	}{
		Context:       cfg.DecisionEngine.ContextAnalysis,
		Custom:        cfg.Lexicon.CustomWords,
		Fuzzy:         cfg.Lexicon.Fuzzy,
		Combo:         cfg.CombinationRules,
		HarmfulPolicy: cfg.SemanticRules.HarmfulValuePolicy,
	})
	if err != nil {
		return riskTermIndex{}, fmt.Errorf("encode local context policy: %w", err)
	}
	signature := string(encoded)

	a.mu.RLock()
	if a.signature == signature {
		index := a.index
		a.mu.RUnlock()
		return index, nil
	}
	a.mu.RUnlock()

	terms := append([]riskTerm(nil), a.builtins...)
	appendConfiguredTerms := func(values map[string][]string) {
		for category, words := range values {
			for _, word := range words {
				if term, ok := newRiskTermWithRole(word, category, CandidateRoleConcept); ok {
					terms = append(terms, term)
				}
			}
		}
	}
	appendConfiguredTerms(cfg.Lexicon.CustomWords.Block)
	appendConfiguredTerms(cfg.Lexicon.CustomWords.Review)
	appendConfiguredTerms(cfg.Lexicon.Fuzzy.CandidateWords)
	appendConfiguredTerms(cfg.DecisionEngine.ContextAnalysis.RiskConcepts)
	if !cfg.SemanticRules.HarmfulValuePolicy.Disabled {
		harmfulConcepts := append([]string(nil), cfg.SemanticRules.HarmfulValuePolicy.SelfHarmActions...)
		harmfulConcepts = append(harmfulConcepts, cfg.SemanticRules.HarmfulValuePolicy.DeathWishActions...)
		harmfulConcepts = append(harmfulConcepts, cfg.SemanticRules.HarmfulValuePolicy.DangerousActions...)
		harmfulConcepts = append(harmfulConcepts, cfg.SemanticRules.HarmfulValuePolicy.DangerousSubstances...)
		for _, concept := range harmfulConcepts {
			if term, ok := newRiskTermWithRole(concept, "harmful_value", CandidateRoleConcept); ok {
				terms = append(terms, term)
			}
		}
	}
	for _, rule := range cfg.CombinationRules {
		category := strings.TrimSpace(rule.Category)
		if category == "" {
			category = strings.TrimSpace(rule.ID)
		}
		for _, subject := range rule.Subjects {
			if term, ok := newRiskTermWithRole(subject, category, CandidateRoleSubject); ok {
				term.AllowPureASCII = false
				terms = append(terms, term)
			}
		}
		for _, predicate := range rule.Predicates {
			if term, ok := newRiskTermWithRole(predicate, category, CandidateRolePredicate); ok {
				term.AllowPureASCII = false
				terms = append(terms, term)
			}
		}
	}
	index := buildRiskTermIndex(terms)

	a.mu.Lock()
	a.signature = signature
	a.index = index
	a.mu.Unlock()
	return index, nil
}

func loadBuiltinRiskTerms() ([]riskTerm, error) {
	loader := dictionary.NewLoader()
	if err := loader.LoadDefaultWords(context.Background()); err != nil {
		return nil, err
	}
	terms := make([]riskTerm, 0, 160)
	for word, category := range loader.GetWords() {
		if category == swdcategory.None {
			continue
		}
		if term, ok := newRiskTerm(word, swdCategoryName(category)); ok {
			terms = append(terms, term)
		}
	}
	return terms, nil
}

func newRiskTerm(value, category string) (riskTerm, bool) {
	return newRiskTermWithRole(value, category, CandidateRoleConcept)
}

func newRiskTermWithRole(value, category, role string) (riskTerm, bool) {
	compact := compactText(normalizeText(value))
	category = strings.ToLower(strings.TrimSpace(category))
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = CandidateRoleConcept
	}
	runeCount := len([]rune(compact))
	if compact == "" || category == "" || runeCount < 2 || runeCount > 12 || !containsHan(compact) {
		return riskTerm{}, false
	}
	full := phoneticSignature(compact, pinyin.Normal)
	initials := phoneticSignature(compact, pinyin.FirstLetter)
	if full == "" || initials == "" {
		return riskTerm{}, false
	}
	return riskTerm{
		Text:            strings.TrimSpace(value),
		Category:        category,
		Role:            role,
		Compact:         compact,
		Pinyin:          full,
		Initials:        initials,
		RuneCount:       runeCount,
		PinyinPattern:   compilePhoneticVariantPattern(compact, pinyin.Normal),
		InitialsPattern: compilePhoneticVariantPattern(compact, pinyin.FirstLetter),
		AllowPureASCII:  true,
	}, true
}

// compilePhoneticVariantPattern derives mixed Han/pinyin forms from the
// canonical risk term itself. It deliberately contains no observed-form map:
// every Han rune may remain literal or be represented by its local pinyin.
func compilePhoneticVariantPattern(value string, style int) *regexp.Regexp {
	var pattern strings.Builder
	for _, r := range value {
		literal := regexp.QuoteMeta(string(r))
		if !unicode.Is(unicode.Han, r) {
			pattern.WriteString(literal)
			continue
		}
		phonetic := phoneticSignature(string(r), style)
		if phonetic == "" || phonetic == string(r) {
			pattern.WriteString(literal)
			continue
		}
		pattern.WriteString("(?:")
		pattern.WriteString(literal)
		pattern.WriteByte('|')
		pattern.WriteString(regexp.QuoteMeta(phonetic))
		pattern.WriteByte(')')
	}
	if pattern.Len() == 0 {
		return nil
	}
	return regexp.MustCompile(pattern.String())
}

func buildRiskTermIndex(terms []riskTerm) riskTermIndex {
	index := riskTermIndex{
		byPinyin:      make(map[string][]riskTerm),
		byASCIIPinyin: make(map[string][]riskTerm),
		byInitials:    make(map[string][]riskTerm),
		byLength:      make(map[int][]riskTerm),
	}
	seen := make(map[string]struct{}, len(terms))
	lengthSet := make(map[int]struct{})
	for _, term := range terms {
		key := term.Category + "\x00" + term.Role + "\x00" + term.Compact
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		index.terms = append(index.terms, term)
		index.byPinyin[term.Pinyin] = append(index.byPinyin[term.Pinyin], term)
		if term.AllowPureASCII {
			index.byASCIIPinyin[term.Pinyin] = append(index.byASCIIPinyin[term.Pinyin], term)
			index.byInitials[term.Initials] = append(index.byInitials[term.Initials], term)
		}
		index.byLength[term.RuneCount] = append(index.byLength[term.RuneCount], term)
		lengthSet[term.RuneCount] = struct{}{}
	}
	for length := range lengthSet {
		index.lengths = append(index.lengths, length)
	}
	sort.Ints(index.lengths)
	sort.SliceStable(index.terms, func(i, j int) bool {
		if index.terms[i].RuneCount != index.terms[j].RuneCount {
			return index.terms[i].RuneCount > index.terms[j].RuneCount
		}
		if index.terms[i].Category != index.terms[j].Category {
			return index.terms[i].Category < index.terms[j].Category
		}
		return index.terms[i].Compact < index.terms[j].Compact
	})
	return index
}

func localVariantCandidates(ctx context.Context, text NormalizedComment, index riskTermIndex,
	maxCandidates int,
) ([]RewriteCandidate, error) {
	emojiIndex, err := resolveEmojiAnnotationIndex()
	if err != nil {
		return nil, err
	}
	candidates := make([]RewriteCandidate, 0, maxCandidates)
	seen := make(map[string]struct{})
	appendMatches := func(observed, method string, confidence float64, clause int, terms []riskTerm) {
		for _, term := range terms {
			if len(candidates) >= maxCandidates || observed == term.Compact {
				continue
			}
			key := observed + "\x00" + term.Category + "\x00" + term.Role + "\x00" + term.Compact
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, RewriteCandidate{
				Text:       term.Text,
				Observed:   observed,
				Category:   term.Category,
				Role:       term.Role,
				Method:     method,
				Confidence: confidence,
				Ambiguous:  true,
				Rationale:  "由本地风险词库的拼音索引生成",
				Clause:     clause,
			})
		}
	}

	for clauseIndex, clause := range semanticClauses(text) {
		if ctx.Err() != nil || len(candidates) >= maxCandidates {
			break
		}
		for _, candidate := range emojiVariantCandidates(ctx, clause, clauseIndex+1, index, emojiIndex,
			maxCandidates-len(candidates)) {
			key := candidate.Observed + "\x00" + candidate.Category + "\x00" + candidate.Role + "\x00" +
				compactText(normalizeText(candidate.Text))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, candidate)
		}
		for _, token := range localASCIIWordRegexp.FindAllString(clause.Compact, -1) {
			appendMatches(token, "pinyin_initials", initialsCandidateConfidence(token), clauseIndex+1,
				index.byInitials[token])
			appendMatches(token, "pinyin_full", 0.9, clauseIndex+1, index.byASCIIPinyin[token])
		}
		for _, term := range index.terms {
			if len(candidates) >= maxCandidates {
				break
			}
			if term.PinyinPattern != nil {
				observed := term.PinyinPattern.FindString(clause.Compact)
				if observed != "" {
					appendMatches(observed, "pinyin_full", 0.9, clauseIndex+1, []riskTerm{term})
				}
			}
			if term.InitialsPattern != nil {
				observed := term.InitialsPattern.FindString(clause.Compact)
				// Pure ASCII initials are already handled by the exact index above.
				// Requiring a literal Han rune prevents short initials from matching
				// arbitrary substrings inside longer Latin words.
				if observed != "" && containsHan(observed) {
					appendMatches(observed, "pinyin_initials", 0.82, clauseIndex+1, []riskTerm{term})
				}
			}
		}
		for _, run := range localHanRunRegexp.FindAllString(clause.Compact, -1) {
			runes := []rune(run)
			for _, length := range index.lengths {
				if length > len(runes) || len(candidates) >= maxCandidates {
					continue
				}
				for start := 0; start+length <= len(runes) && len(candidates) < maxCandidates; start++ {
					observed := string(runes[start : start+length])
					signature := phoneticSignature(observed, pinyin.Normal)
					appendMatches(observed, "pinyin_homophone", 0.92, clauseIndex+1,
						index.byPinyin[signature])
					if length < 3 {
						continue
					}
					for _, term := range index.byLength[length] {
						if weightedEditDistance([]rune(signature), []rune(term.Pinyin)) == 2 {
							appendMatches(observed, "pinyin_near", 0.78, clauseIndex+1, []riskTerm{term})
						}
					}
				}
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Confidence > candidates[j].Confidence
	})
	return candidates, nil
}

func applySurroundingContext(candidates []RewriteCandidate, req Request) {
	surrounding := compactText(normalizeText(strings.Join([]string{
		req.ArticleTitle,
		req.ArticleCategory,
		req.ParentContent,
		req.ReplyToContent,
	}, " ")))
	if surrounding == "" {
		return
	}
	for index := range candidates {
		canonical := compactText(normalizeText(candidates[index].Text))
		if canonical == "" || !strings.Contains(surrounding, canonical) {
			continue
		}
		if candidates[index].Confidence < 0.9 {
			candidates[index].Confidence = 0.9
		}
		candidates[index].Rationale = "本地拼音索引候选，并由文章或回复上下文中的规范词印证"
	}
}

func candidateEvidence(candidates []RewriteCandidate, relations []SemanticRelation,
	cfg appconfig.CommentModerationConfig,
) []Evidence {
	evidence := make([]Evidence, 0, len(candidates))
	for index, candidate := range candidates {
		relation, hasRelation := candidateSemanticRelation(candidate, relations)
		if hasRelation && relationIsCounterEvidence(relation) {
			continue
		}
		confidence := candidate.Confidence
		if candidate.Category == "abuse" &&
			(!hasRelation || relation.Action != "辱骂" && relation.Action != "严重攻击") {
			if !containsAnyNormalized(compactText(normalizeText(candidate.Text)),
				cfg.SemanticRules.AbusePolicy.SevereMarkers) {
				continue
			}
			confidence = minProbability(confidence, 0.62)
		}
		if candidate.Role == CandidateRoleSubject || candidate.Role == CandidateRolePredicate {
			if !hasRelation || relation.Action == "" || relation.Object == "" {
				continue
			}
		}
		if candidate.Method == "pinyin_near" && (!hasRelation || relation.Action == "") &&
			!strings.Contains(candidate.Rationale, "上下文中的规范词印证") {
			continue
		}
		if candidate.Method == "pinyin_homophone" && hasRelation && relation.Action != "辱骂" &&
			relationActionIsOnlyGeneric(relation.Action, cfg) &&
			!strings.Contains(candidate.Rationale, "上下文中的规范词印证") {
			continue
		}
		if candidate.Method == "pinyin_initials" && containsHan(candidate.Observed) &&
			(!hasRelation || relation.Action == "") &&
			!hasCorroboratingInitialCandidate(candidate, candidates) {
			continue
		}
		evidence = append(evidence, Evidence{
			ID:         fmt.Sprintf("local-%03d", index+1),
			Source:     SourceLocalContext,
			Category:   candidate.Category,
			Polarity:   "positive",
			Confidence: confidence,
			CorrelationGroup: fmt.Sprintf("clause:%d:%s:%s", candidate.Clause, candidate.Category,
				compactText(normalizeText(candidate.Text))),
			Value:  candidate.Observed + "→" + candidate.Text,
			RuleID: candidate.Method,
			Clause: candidate.Clause,
		})
	}
	return evidence
}

func hasCorroboratingInitialCandidate(candidate RewriteCandidate, candidates []RewriteCandidate) bool {
	for _, other := range candidates {
		if other.Clause == candidate.Clause && other.Category == candidate.Category &&
			other.Role == candidate.Role && other.Method == candidate.Method &&
			other.Observed != candidate.Observed {
			return true
		}
	}
	return false
}

func relationActionIsOnlyGeneric(action string, cfg appconfig.CommentModerationConfig) bool {
	for _, marker := range resolvedRelationVocabulary(cfg).FirstPersonMarkers {
		if action == compactText(normalizeText(marker)) {
			return true
		}
	}
	return action == ""
}

func candidateSemanticRelation(candidate RewriteCandidate, relations []SemanticRelation) (SemanticRelation, bool) {
	for _, relation := range relations {
		if relation.Clause != candidate.Clause || relation.Category != candidate.Category {
			continue
		}
		if strings.Contains(relation.Evidence, candidate.Observed) ||
			strings.Contains(relation.Evidence, candidate.Text) || relation.Action == candidate.Text ||
			relation.Object == candidate.Text ||
			relation.Result == candidate.Text {
			return relation, true
		}
	}
	return SemanticRelation{}, false
}

type localIntentProfile struct {
	intent            string
	confidence        float64
	benignProbability float64
	boost             float64
	explanation       string
}

func phoneticSignature(value string, style int) string {
	args := pinyin.NewArgs()
	args.Style = style
	args.Fallback = func(r rune, _ pinyin.Args) []string {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return []string{strings.ToLower(string(r))}
		}
		return nil
	}
	return strings.ToLower(strings.Join(pinyin.LazyPinyin(value, args), ""))
}

func initialsCandidateConfidence(token string) float64 {
	switch len([]rune(token)) {
	case 2:
		return 0.76
	case 3:
		return 0.82
	default:
		return 0.86
	}
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
