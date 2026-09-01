package moderation

import "time"

const (
	LevelAllow  = "allow"
	LevelNotice = "notice"
	LevelReview = "review"
	LevelBlock  = "block"
)

const (
	SourceLexicon      = "lexicon"
	SourceStructure    = "structure"
	SourceContext      = "context"
	SourceBehavior     = "behavior"
	SourceSemantic     = "semantic"
	SourceSimilarity   = "similarity"
	SourceLocalContext = "local_context"
)

const (
	CandidateRoleConcept   = "concept"
	CandidateRoleSubject   = "subject"
	CandidateRolePredicate = "predicate"
)

const (
	defaultUserWindowSeconds        int64 = 600
	defaultIPWindowSeconds          int64 = 600
	defaultDuplicateWindowSeconds   int64 = 86400
	defaultUserReviewThreshold      int64 = 6
	defaultIPReviewThreshold        int64 = 12
	defaultDuplicateReviewThreshold int64 = 2
	defaultDuplicateBlockThreshold  int64 = 4
	defaultNearDuplicateWindow      int64 = 86400
	defaultNearDuplicateThreshold   int64 = 2
	defaultNearDuplicateDistance          = 10
	defaultNearDuplicateMinRunes          = 12
	defaultNearDuplicateMaxSamples  int64 = 100
	defaultNearDuplicateLengthDiff        = 30
	defaultFuzzyMaxDistance               = 1
	defaultFuzzyMinWordRunes              = 4
	behaviorTTLExtra                      = time.Minute
	base64MinLength                       = 16
	decodedURLReasonMaxLen                = 80
)

type Request struct {
	CommentID uint64
	UserID    uint64
	ArticleID uint64
	ClientIP  string
	Content   string
	// The following fields are optional moderation context. They must never be
	// concatenated into Content or fed to the deterministic detectors.
	ArticleTitle    string
	ArticleCategory string
	ParentContent   string
	ReplyToContent  string
	Now             time.Time
}

type Result struct {
	Status   string
	Score    int
	Signals  []Signal
	Reasons  []string
	Decision string
	Trace    Trace
}

type Trace struct {
	Clauses           []ClauseTrace
	DetectorSignals   []Signal
	SuppressedSignals []Signal
	Behavior          BehaviorTrace
	DecisionEngine    *DecisionEngineTrace
	Decisions         DecisionFlowTrace
}

// RewriteCandidate is an interpretation candidate, not a destructive rewrite.
// Keeping the original text and all candidates prevents an ambiguous abbreviation
// such as "ltp" from being assigned one meaning before context is considered.
type RewriteCandidate struct {
	Text       string  `json:"text"`
	Observed   string  `json:"observed,omitempty"`
	Category   string  `json:"category,omitempty"`
	Role       string  `json:"role,omitempty"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	Ambiguous  bool    `json:"ambiguous"`
	Rationale  string  `json:"rationale,omitempty"`
	Clause     int     `json:"clauseID,omitempty"`
}

// Evidence is the common currency of the evidence-based decision pipeline.
// CorrelationGroup identifies observations that came from the same underlying
// phrase so they are not counted repeatedly as independent proof.
type Evidence struct {
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Category         string  `json:"category"`
	Polarity         string  `json:"polarity"`
	Confidence       float64 `json:"confidence"`
	CorrelationGroup string  `json:"correlationGroup"`
	Value            string  `json:"value,omitempty"`
	RuleID           string  `json:"ruleID,omitempty"`
	Clause           int     `json:"clauseID,omitempty"`
}

type ContextAssessment struct {
	Analyzed              bool
	Confidence            float64
	Intent                string
	BenignProbability     float64
	CategoryProbabilities map[string]float64
	Candidates            []RewriteCandidate
	Evidence              []Evidence
	Relations             []SemanticRelation
	Explanation           string
	UnavailableReason     string
}

type EvidenceDeduplication struct {
	Discarded Evidence `json:"discarded"`
	KeptID    string   `json:"keptID"`
	Reason    string   `json:"reason"`
}

const (
	RelationTypeAction     = "action"
	RelationTypeEvaluation = "evaluation"
	RelationTypeExpression = "expression"
)

const (
	RelationStanceActionable   = "actionable"
	RelationStanceCondemnation = "condemnation"
	RelationStanceWarning      = "warning"
	RelationStanceRejection    = "rejection"
	RelationStanceReporting    = "reporting"
	RelationStanceSelfConcern  = "self_concern"
)

// SemanticRelation is a clause-scoped interpretation of either an action or
// an evaluation. Action relations answer "who does what to which object";
// evaluation relations answer "which behaviour is judged as what, and from
// which stance". Keeping the two structures distinct prevents a risk concept
// mentioned in a condemnation from being mistaken for the speaker's action.
type SemanticRelation struct {
	ID         string  `json:"id"`
	Clause     int     `json:"clauseID"`
	Type       string  `json:"type,omitempty"`
	Subject    string  `json:"subject,omitempty"`
	Action     string  `json:"action,omitempty"`
	Object     string  `json:"object,omitempty"`
	Predicate  string  `json:"predicate,omitempty"`
	Result     string  `json:"result,omitempty"`
	Stance     string  `json:"stance,omitempty"`
	Category   string  `json:"category,omitempty"`
	Subtype    string  `json:"subtype,omitempty"`
	Evidence   string  `json:"evidence,omitempty"`
	Negated    bool    `json:"negated"`
	Quoted     bool    `json:"quoted"`
	Reported   bool    `json:"reported"`
	Inferred   bool    `json:"inferred"`
	Confidence float64 `json:"confidence"`
}

type ProbabilityDecision struct {
	Status                string
	RiskProbability       float64
	Confidence            float64
	Decision              string
	Calibration           string
	CategoryProbabilities map[string]float64
	Actionable            bool
	FallbackReason        string
}

type ProbabilityThresholdTrace struct {
	ApproveMax    float64 `json:"approveMax"`
	RejectMin     float64 `json:"rejectMin"`
	MinConfidence float64 `json:"minConfidence"`
}

type CategoryFusionTrace struct {
	Category        string  `json:"category"`
	RuleRisk        float64 `json:"ruleRisk"`
	ContextRisk     float64 `json:"contextRisk"`
	ContextCovered  bool    `json:"contextCovered"`
	ContextWeight   float64 `json:"contextWeight"`
	ContentRisk     float64 `json:"contentRisk"`
	BehaviorRisk    float64 `json:"behaviorRisk"`
	CounterEvidence float64 `json:"counterEvidence"`
	FinalRisk       float64 `json:"finalRisk"`
}

type EvidenceFusionTrace struct {
	Thresholds   ProbabilityThresholdTrace `json:"thresholds"`
	Categories   []CategoryFusionTrace     `json:"categories,omitempty"`
	InputCount   int                       `json:"inputCount"`
	OutputCount  int                       `json:"outputCount"`
	Deduplicated []EvidenceDeduplication   `json:"deduplicated,omitempty"`
}

type DecisionEngineTrace struct {
	Candidates []RewriteCandidate
	Evidence   []Evidence
	Context    ContextAssessment
	Fusion     EvidenceFusionTrace
	Decision   ProbabilityDecision
}

type DecisionSnapshot struct {
	Status   string `json:"status"`
	Score    int    `json:"score"`
	Decision string `json:"decision"`
}

type DecisionApplicationTrace struct {
	Evaluated bool             `json:"evaluated"`
	Applied   bool             `json:"applied"`
	Before    DecisionSnapshot `json:"before"`
	Candidate DecisionSnapshot `json:"candidate"`
	After     DecisionSnapshot `json:"after"`
	Reason    string           `json:"reason,omitempty"`
}

type HardSafetyTrace struct {
	Evaluated bool             `json:"evaluated"`
	Triggered bool             `json:"triggered"`
	RuleID    string           `json:"ruleID,omitempty"`
	Before    DecisionSnapshot `json:"before"`
	After     DecisionSnapshot `json:"after"`
	Reason    string           `json:"reason,omitempty"`
}

type FeedbackApplicationTrace struct {
	Evaluated         bool             `json:"evaluated"`
	Matched           bool             `json:"matched"`
	Consensus         bool             `json:"consensus"`
	Applied           bool             `json:"applied"`
	Scope             string           `json:"scope,omitempty"`
	Support           int64            `json:"support"`
	Total             int64            `json:"total"`
	Conflicts         int64            `json:"conflicts"`
	SimulationSupport int64            `json:"simulationSupport"`
	LiveSupport       int64            `json:"liveSupport"`
	ExpectedStatus    string           `json:"expectedStatus,omitempty"`
	ExpectedCategory  string           `json:"expectedCategory,omitempty"`
	Before            DecisionSnapshot `json:"before"`
	After             DecisionSnapshot `json:"after"`
	Reason            string           `json:"reason,omitempty"`
}

type DecisionFlowTrace struct {
	Rule        DecisionSnapshot         `json:"rule"`
	Probability DecisionApplicationTrace `json:"probability"`
	HardSafety  HardSafetyTrace          `json:"hardSafety"`
	Feedback    FeedbackApplicationTrace `json:"feedback"`
	Final       DecisionSnapshot         `json:"final"`
}

type ClauseTrace struct {
	ID   int
	Text NormalizedComment
}

type Signal struct {
	Source   string
	Category string
	Level    string
	Score    int
	Reason   string
	Evidence string
	RuleID   string
	Clause   int
}

type NormalizedComment struct {
	Raw          string
	Normalized   string
	Compact      string
	Confusable   string
	DecodedTexts []string
}

func (n NormalizedComment) Views() []string {
	views := []string{n.Normalized, n.Compact}
	if n.Confusable != "" && n.Confusable != n.Compact {
		views = append(views, n.Confusable)
	}
	views = append(views, n.DecodedTexts...)
	return views
}

type BehaviorState struct {
	UserCount              int64
	IPCount                int64
	DuplicateCount         int64
	NearDuplicateCount     int64
	UserEvaluated          bool
	IPEvaluated            bool
	DuplicateEvaluated     bool
	NearDuplicateEvaluated bool
}

type BehaviorMetricTrace struct {
	Name             string `json:"name"`
	Evaluated        bool   `json:"evaluated"`
	ObservedCount    int64  `json:"observedCount"`
	ProspectiveCount int64  `json:"prospectiveCount"`
	WindowSeconds    int64  `json:"windowSeconds"`
	ReviewThreshold  int64  `json:"reviewThreshold"`
	BlockThreshold   int64  `json:"blockThreshold,omitempty"`
	TriggeredLevel   string `json:"triggeredLevel,omitempty"`
	SkippedReason    string `json:"skippedReason,omitempty"`
}

type BehaviorTrace struct {
	Status            string                `json:"status"`
	ReadOnly          bool                  `json:"readOnly"`
	ContextProvided   bool                  `json:"contextProvided"`
	UnavailableReason string                `json:"unavailableReason,omitempty"`
	Metrics           []BehaviorMetricTrace `json:"metrics,omitempty"`
}

type BehaviorEvaluation struct {
	Signals []Signal
	Trace   BehaviorTrace
}

type BehaviorKeys struct {
	User          string
	IP            string
	Duplicate     string
	NearDuplicate string
}
