package moderation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

const (
	bootstrapCalibration = "bootstrap-uncalibrated"
)

func (m *Moderator) evaluateDecisionEngine(ctx context.Context, req Request, text NormalizedComment,
	signals []Signal, cfg appconfig.CommentModerationConfig,
) *DecisionEngineTrace {
	candidates := rewriteCandidates(text)
	evidence := evidenceFromSignals(signals, cfg)
	assessment := ContextAssessment{UnavailableReason: "context_analyzer_unavailable"}
	if m != nil && m.contextAnalyzer != nil {
		var err error
		assessment, err = m.contextAnalyzer.Analyze(ctx, ContextInput{
			Request:    req,
			Text:       text,
			Candidates: candidates,
			Evidence:   evidence,
		}, cfg)
		if err != nil {
			assessment = ContextAssessment{UnavailableReason: "context_analysis_error"}
			if m.logger != nil {
				m.logger.Warn("comment local context analysis failed", zap.Error(err))
			}
		}
	}
	candidates = mergeRewriteCandidates(candidates, assessment.Candidates)
	inputCount := len(evidence) + len(assessment.Evidence)
	evidence, deduplicated := mergeEvidenceWithTrace(evidence, assessment.Evidence)
	decision, fusion := fuseEvidenceWithTrace(evidence, assessment, cfg.DecisionEngine)
	fusion.InputCount = inputCount
	fusion.OutputCount = len(evidence)
	fusion.Deduplicated = deduplicated
	return &DecisionEngineTrace{
		Candidates: candidates,
		Evidence:   evidence,
		Context:    assessment,
		Fusion:     fusion,
		Decision:   decision,
	}
}

func evidenceFromSignals(signals []Signal, configs ...appconfig.CommentModerationConfig) []Evidence {
	var cfg appconfig.CommentModerationConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	byGroup := make(map[string]Evidence, len(signals))
	for index, signal := range signals {
		category := strings.ToLower(strings.TrimSpace(signal.Category))
		if category == "" {
			category = "uncategorized"
		}
		polarity := "positive"
		if normalizeLevel(signal.Level) == LevelAllow {
			polarity = "negative"
		}
		value := strings.TrimSpace(signal.Evidence)
		if value == "" {
			value = strings.TrimSpace(signal.RuleID)
		}
		group := evidenceCorrelationGroup(signal, category, value)
		item := Evidence{
			ID:               fmt.Sprintf("e%03d", index+1),
			Source:           strings.TrimSpace(signal.Source),
			Category:         category,
			Polarity:         polarity,
			Confidence:       bootstrapSignalConfidence(signal, cfg),
			CorrelationGroup: group,
			Value:            value,
			RuleID:           strings.TrimSpace(signal.RuleID),
			Clause:           signal.Clause,
		}
		key := category + "\x00" + polarity + "\x00" + group
		if current, exists := byGroup[key]; !exists || item.Confidence > current.Confidence {
			byGroup[key] = item
		}
	}
	evidence := make([]Evidence, 0, len(byGroup))
	for _, item := range byGroup {
		evidence = append(evidence, item)
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Clause != evidence[j].Clause {
			return evidence[i].Clause < evidence[j].Clause
		}
		if evidence[i].Category != evidence[j].Category {
			return evidence[i].Category < evidence[j].Category
		}
		return evidence[i].ID < evidence[j].ID
	})
	return evidence
}

func mergeEvidence(groups ...[]Evidence) []Evidence {
	result, _ := mergeEvidenceWithTrace(groups...)
	return result
}

func mergeEvidenceWithTrace(groups ...[]Evidence) ([]Evidence, []EvidenceDeduplication) {
	byGroup := make(map[string]Evidence)
	deduplicated := make([]EvidenceDeduplication, 0)
	for _, group := range groups {
		for _, item := range group {
			key := item.Category + "\x00" + item.Polarity + "\x00" + item.CorrelationGroup
			if current, exists := byGroup[key]; !exists {
				byGroup[key] = item
			} else if item.Confidence > current.Confidence {
				deduplicated = append(deduplicated, EvidenceDeduplication{
					Discarded: current, KeptID: item.ID, Reason: "stronger_correlated_evidence",
				})
				byGroup[key] = item
			} else {
				deduplicated = append(deduplicated, EvidenceDeduplication{
					Discarded: item, KeptID: current.ID, Reason: "weaker_correlated_evidence",
				})
			}
		}
	}
	result := make([]Evidence, 0, len(byGroup))
	for _, item := range byGroup {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Clause != result[j].Clause {
			return result[i].Clause < result[j].Clause
		}
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].ID < result[j].ID
	})
	return result, deduplicated
}

func evidenceCorrelationGroup(signal Signal, category, value string) string {
	canonical := compactText(normalizeText(value))
	if canonical == "" {
		canonical = canonicalKeyPart(signal.RuleID)
	}
	if canonical == "" {
		canonical = canonicalKeyPart(signal.Reason)
	}
	return fmt.Sprintf("clause:%d:%s:%s", signal.Clause, category, canonical)
}

// These are intentionally identified as bootstrap strengths, not calibrated
// probabilities. A later offline calibration artifact can replace this function
// without changing detectors or the fusion contract.
func bootstrapSignalConfidence(signal Signal, cfg appconfig.CommentModerationConfig) float64 {
	calibration := resolvedCalibration(cfg)
	if normalizeLevel(signal.Level) == LevelAllow {
		return calibration.Allow
	}
	if normalizeLevel(signal.Level) == LevelBlock {
		if signal.Source == SourceStructure && signal.RuleID == "script_injection" {
			return calibration.ScriptInjectionBlock
		}
		return calibration.Block
	}
	if confidence := calibration.Sources[signal.Source]; confidence > 0 {
		return confidence
	}
	return calibration.Default
}

// resolvedCalibration keeps a conservative fallback for isolated unit tests and
// failed configuration reloads. Production values come from calibration.yml.
func resolvedCalibration(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationCalibrationConfig {
	calibration := cfg.DecisionEngine.Calibration
	if calibration.Allow > 0 && calibration.Block > 0 && calibration.Default > 0 && len(calibration.Sources) > 0 {
		return calibration
	}
	return appconfig.CommentModerationCalibrationConfig{
		Version: "bootstrap-uncalibrated", Allow: 0.82, Block: 0.9,
		ScriptInjectionBlock: 0.995, Default: 0.58,
		Sources: map[string]float64{
			SourceBehavior: 0.72, SourceContext: 0.66, SourceLexicon: 0.62,
			SourceSimilarity: 0.55, SourceSemantic: 0.7, SourceLocalContext: 0.7,
		},
	}
}

func fuseEvidence(evidence []Evidence, contextAssessment ContextAssessment,
	cfg appconfig.CommentModerationDecisionEngineConfig,
) ProbabilityDecision {
	decision, _ := fuseEvidenceWithTrace(evidence, contextAssessment, cfg)
	return decision
}

func fuseEvidenceWithTrace(evidence []Evidence, contextAssessment ContextAssessment,
	cfg appconfig.CommentModerationDecisionEngineConfig,
) (ProbabilityDecision, EvidenceFusionTrace) {
	type categoryState struct {
		content  []float64
		behavior []float64
		negative []float64
	}
	states := make(map[string]*categoryState)
	globalNegative := make([]float64, 0, 1)
	stateFor := func(category string) *categoryState {
		state := states[category]
		if state == nil {
			state = &categoryState{}
			states[category] = state
		}
		return state
	}
	for _, item := range evidence {
		if item.Polarity == "negative" && (item.Category == "benign_context" || item.Category == "uncategorized") {
			globalNegative = append(globalNegative, item.Confidence)
			continue
		}
		state := stateFor(item.Category)
		if item.Polarity == "negative" {
			state.negative = append(state.negative, item.Confidence)
		} else if item.Source == SourceBehavior {
			state.behavior = append(state.behavior, item.Confidence)
		} else {
			state.content = append(state.content, item.Confidence)
		}
	}
	for category := range contextAssessment.CategoryProbabilities {
		stateFor(category)
	}

	categoryProbabilities := make(map[string]float64, len(states))
	categoryTraces := make([]CategoryFusionTrace, 0, len(states))
	overallRisk := 0.0
	contextWeight := 0.0
	if contextAssessment.Analyzed {
		contextWeight = contextAssessment.Confidence
	}
	for category, state := range states {
		ruleRisk := noisyOr(state.content)
		contentRisk := ruleRisk
		contextRisk := 0.0
		contextCovered := false
		if contextAssessment.Analyzed {
			if rawContextRisk, covered := contextAssessment.CategoryProbabilities[category]; covered {
				contextRisk = clampProbability(rawContextRisk)
				contextCovered = true
				contentRisk = ruleRisk*(1-contextWeight) + contextRisk*contextWeight
			}
		}
		behaviorRisk := noisyOr(state.behavior)
		categoryRisk := noisyOr([]float64{contentRisk, behaviorRisk})
		negativeValues := append([]float64(nil), state.negative...)
		if !contextHasActiveRiskRelation(contextAssessment, category) {
			negativeValues = append(negativeValues, globalNegative...)
		}
		negative := noisyOr(negativeValues)
		if contextAssessment.Analyzed && contextBenignAppliesToCategory(contextAssessment, category) {
			negative = noisyOr([]float64{negative,
				contextAssessment.BenignProbability * contextWeight})
		}
		categoryRisk = clampProbability(categoryRisk * (1 - negative))
		categoryProbabilities[category] = categoryRisk
		categoryTraces = append(categoryTraces, CategoryFusionTrace{
			Category: category, RuleRisk: ruleRisk, ContextRisk: contextRisk,
			ContextCovered: contextCovered, ContextWeight: contextWeight,
			ContentRisk: contentRisk, BehaviorRisk: behaviorRisk,
			CounterEvidence: negative, FinalRisk: categoryRisk,
		})
		if categoryRisk > overallRisk {
			overallRisk = categoryRisk
		}
	}
	sort.Slice(categoryTraces, func(i, j int) bool {
		if categoryTraces[i].FinalRisk != categoryTraces[j].FinalRisk {
			return categoryTraces[i].FinalRisk > categoryTraces[j].FinalRisk
		}
		return categoryTraces[i].Category < categoryTraces[j].Category
	})

	approveMax, rejectMin, minConfidence := probabilityThresholds(cfg.Thresholds)
	status := commentModel.StatusPending
	decisionCode := "probability_review"
	if overallRisk <= approveMax {
		status = commentModel.StatusApproved
		decisionCode = "probability_allow"
	} else if overallRisk >= rejectMin {
		status = commentModel.StatusRejected
		decisionCode = "probability_reject"
	}
	confidence := contextAssessment.Confidence
	if !contextAssessment.Analyzed {
		confidence = maxEvidenceConfidence(evidence)
	}
	actionable := contextAssessment.Analyzed &&
		contextAssessment.Confidence >= minConfidence
	fallbackReason := ""
	switch {
	case !contextAssessment.Analyzed:
		fallbackReason = contextAssessment.UnavailableReason
		if fallbackReason == "" {
			fallbackReason = "context_analysis_unavailable"
		}
	case contextAssessment.Confidence < minConfidence:
		fallbackReason = "context_confidence_below_threshold"
	}
	decision := ProbabilityDecision{
		Status:                status,
		RiskProbability:       overallRisk,
		Confidence:            clampProbability(confidence),
		Decision:              decisionCode,
		Calibration:           calibrationVersion(cfg.Calibration),
		CategoryProbabilities: categoryProbabilities,
		Actionable:            actionable,
		FallbackReason:        fallbackReason,
	}
	return decision, EvidenceFusionTrace{
		Thresholds: ProbabilityThresholdTrace{
			ApproveMax: approveMax, RejectMin: rejectMin, MinConfidence: minConfidence,
		},
		Categories: categoryTraces,
	}
}

func calibrationVersion(cfg appconfig.CommentModerationCalibrationConfig) string {
	if version := strings.TrimSpace(cfg.Version); version != "" {
		return version
	}
	return bootstrapCalibration
}

func contextBenignAppliesToCategory(assessment ContextAssessment, category string) bool {
	if len(assessment.Relations) == 0 {
		return true
	}
	if assessment.Intent == "question" || assessment.Intent == "technical" ||
		assessment.Intent == "reporting" || assessment.Intent == "rejection" ||
		assessment.Intent == "risk_education" || assessment.Intent == "risk_evaluation" {
		return true
	}
	if assessment.Intent == "content_criticism" && category == "abuse" {
		return true
	}
	for _, relation := range assessment.Relations {
		if relation.Category == category && relationIsCounterEvidence(relation) {
			return true
		}
	}
	return false
}

func contextHasActiveRiskRelation(assessment ContextAssessment, category string) bool {
	for _, relation := range assessment.Relations {
		if relation.Category == category && relationIsActionableRisk(relation) {
			return true
		}
	}
	return false
}

func applyDecisionEngine(result Result, trace *DecisionEngineTrace) Result {
	flow := DecisionFlowTrace{Rule: decisionSnapshot(result), Final: decisionSnapshot(result)}
	return applyDecisionEngineWithTrace(result, trace, &flow)
}

func applyDecisionEngineWithTrace(result Result, trace *DecisionEngineTrace,
	flow *DecisionFlowTrace,
) Result {
	before := decisionSnapshot(result)
	if flow != nil {
		flow.Rule = before
		flow.Final = before
		flow.Probability = DecisionApplicationTrace{Before: before, After: before}
		flow.HardSafety = HardSafetyTrace{Before: before, After: before}
	}
	if trace == nil || !trace.Decision.Actionable {
		if flow != nil {
			flow.Probability.Evaluated = trace != nil
			if trace != nil {
				flow.Probability.Candidate = probabilityDecisionSnapshot(trace.Decision)
				flow.Probability.Reason = trace.Decision.FallbackReason
			}
		}
		return result
	}
	candidate := probabilityDecisionSnapshot(trace.Decision)
	if flow != nil {
		flow.Probability.Evaluated = true
		flow.Probability.Candidate = candidate
		flow.HardSafety.Evaluated = true
	}
	if hasHardSafetyEvidence(trace.Evidence) && trace.Decision.Status != commentModel.StatusRejected {
		trace.Decision.Actionable = false
		trace.Decision.FallbackReason = "hard_safety_signal"
		if flow != nil {
			flow.Probability.Reason = "blocked_by_hard_safety"
			flow.HardSafety.Triggered = true
			flow.HardSafety.RuleID = "script_injection"
			flow.HardSafety.Reason = "hard_safety_signal"
		}
		return result
	}
	result.Status = trace.Decision.Status
	result.Score = int(math.Round(trace.Decision.RiskProbability * 100))
	result.Decision = trace.Decision.Decision
	result.Reasons = append(result.Reasons, fmt.Sprintf("decision_engine:risk:%.4f", trace.Decision.RiskProbability))
	if flow != nil {
		flow.Probability.Applied = true
		flow.Probability.After = decisionSnapshot(result)
		flow.HardSafety.Before = flow.Probability.After
		flow.HardSafety.After = flow.Probability.After
		flow.Final = flow.Probability.After
	}
	return result
}

func decisionSnapshot(result Result) DecisionSnapshot {
	return DecisionSnapshot{Status: result.Status, Score: result.Score, Decision: result.Decision}
}

func probabilityDecisionSnapshot(decision ProbabilityDecision) DecisionSnapshot {
	return DecisionSnapshot{
		Status:   decision.Status,
		Score:    int(math.Round(decision.RiskProbability * 100)),
		Decision: decision.Decision,
	}
}

func hasHardSafetyEvidence(evidence []Evidence) bool {
	for _, item := range evidence {
		if item.Source == SourceStructure && item.RuleID == "script_injection" && item.Polarity == "positive" {
			return true
		}
	}
	return false
}

func noisyOr(values []float64) float64 {
	remaining := 1.0
	for _, value := range values {
		remaining *= 1 - clampProbability(value)
	}
	return clampProbability(1 - remaining)
}

func maxEvidenceConfidence(evidence []Evidence) float64 {
	maximum := 0.0
	for _, item := range evidence {
		if item.Confidence > maximum {
			maximum = item.Confidence
		}
	}
	return maximum
}

func probabilityThresholds(cfg appconfig.CommentModerationProbabilityThresholdConfig) (float64, float64, float64) {
	approveMax := cfg.ApproveMax
	if approveMax <= 0 || approveMax >= 1 {
		approveMax = 0.2
	}
	rejectMin := cfg.RejectMin
	if rejectMin <= approveMax || rejectMin > 1 {
		rejectMin = 0.9
	}
	minConfidence := cfg.MinConfidence
	if minConfidence <= 0 || minConfidence > 1 {
		minConfidence = 0.7
	}
	return approveMax, rejectMin, minConfidence
}

func clampProbability(value float64) float64 {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	if value >= 1 {
		return 1
	}
	return value
}
