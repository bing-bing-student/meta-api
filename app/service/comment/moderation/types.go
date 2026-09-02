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

// Request 描述一次评论审核请求，同时携带行为统计和上下文分析所需的可选信息。
type Request struct {
	CommentID uint64
	UserID    uint64
	ArticleID uint64
	ClientIP  string
	Content   string
	// 以下字段仅用于语义上下文，不得拼接到 Content 或传给确定性检测器。
	ArticleTitle    string
	ArticleCategory string
	ParentContent   string
	ReplyToContent  string
	Now             time.Time
}

// Result 保存审核状态、风险分、规则信号和完整决策轨迹。
type Result struct {
	Status   string
	Score    int
	Signals  []Signal
	Reasons  []string
	Decision string
	Trace    Trace
}

// Trace 聚合分句、检测、抑制、行为和概率决策各阶段的可观测数据。
type Trace struct {
	Clauses           []ClauseTrace
	DetectorSignals   []Signal
	SuppressedSignals []Signal
	Behavior          BehaviorTrace
	DecisionEngine    *DecisionEngineTrace
	Decisions         DecisionFlowTrace
}

// RewriteCandidate 表示一个非破坏性的文本解释候选；保留原文和全部候选，
// 可避免在上下文分析前就将“ltp”等歧义缩写强制绑定为单一含义。
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

// Evidence 是决策链中的统一证据单元；CorrelationGroup 标记来自同一原始短语的观测，
// 防止这些相关信号被当作多份独立证据重复计分。
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

// ContextAssessment 描述本地上下文分析的意图、概率、候选词、证据和语义关系。
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

// EvidenceDeduplication 记录一次相关证据去重的丢弃项、保留项及原因。
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

// SemanticRelation 表示分句范围内的动作、评价或表达关系。它区分“谁对什么做了什么”
// 和“评论者如何评价某个行为”，避免将批判语境中的风险词误当成评论者的行动。
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

// ProbabilityDecision 保存证据融合后的风险概率、置信度、处置状态和降级原因。
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

// ProbabilityThresholdTrace 记录本次决策实际使用的通过、拒绝和最低置信度阈值。
type ProbabilityThresholdTrace struct {
	ApproveMax    float64 `json:"approveMax"`
	RejectMin     float64 `json:"rejectMin"`
	MinConfidence float64 `json:"minConfidence"`
}

// CategoryFusionTrace 展示单个风险分类中规则、上下文、行为和反证的融合过程。
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

// EvidenceFusionTrace 记录证据融合的阈值、分类详情、输入输出数量和去重明细。
type EvidenceFusionTrace struct {
	Thresholds   ProbabilityThresholdTrace `json:"thresholds"`
	Categories   []CategoryFusionTrace     `json:"categories,omitempty"`
	InputCount   int                       `json:"inputCount"`
	OutputCount  int                       `json:"outputCount"`
	Deduplicated []EvidenceDeduplication   `json:"deduplicated,omitempty"`
}

// DecisionEngineTrace 聚合本地候选、证据、上下文、融合过程和概率决策。
type DecisionEngineTrace struct {
	Candidates []RewriteCandidate
	Evidence   []Evidence
	Context    ContextAssessment
	Fusion     EvidenceFusionTrace
	Decision   ProbabilityDecision
}

// DecisionSnapshot 是决策链某一时点的状态、分值和决策代码快照。
type DecisionSnapshot struct {
	Status   string `json:"status"`
	Score    int    `json:"score"`
	Decision string `json:"decision"`
}

// DecisionApplicationTrace 记录某个决策阶段是否评估、是否应用及应用前后的快照。
type DecisionApplicationTrace struct {
	Evaluated bool             `json:"evaluated"`
	Applied   bool             `json:"applied"`
	Before    DecisionSnapshot `json:"before"`
	Candidate DecisionSnapshot `json:"candidate"`
	After     DecisionSnapshot `json:"after"`
	Reason    string           `json:"reason,omitempty"`
}

// HardSafetyTrace 记录不可降级的硬安全规则是否触发及其对结果的覆盖。
type HardSafetyTrace struct {
	Evaluated bool             `json:"evaluated"`
	Triggered bool             `json:"triggered"`
	RuleID    string           `json:"ruleID,omitempty"`
	Before    DecisionSnapshot `json:"before"`
	After     DecisionSnapshot `json:"after"`
	Reason    string           `json:"reason,omitempty"`
}

// FeedbackApplicationTrace 记录人工校准的匹配、共识、数据来源和应用结果。
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

// DecisionFlowTrace 按先后顺序保存规则、概率、硬安全、人工反馈和最终决策。
type DecisionFlowTrace struct {
	Rule        DecisionSnapshot         `json:"rule"`
	Probability DecisionApplicationTrace `json:"probability"`
	HardSafety  HardSafetyTrace          `json:"hardSafety"`
	Feedback    FeedbackApplicationTrace `json:"feedback"`
	Final       DecisionSnapshot         `json:"final"`
}

// ClauseTrace 将分句编号与对应的归一化文本关联起来。
type ClauseTrace struct {
	ID   int
	Text NormalizedComment
}

// Signal 表示检测器产生的原始审核信号，包含来源、分类、等级和命中证据。
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

// NormalizedComment 保存原文、归一化文本、紧凑文本、混淆骨架和解码出的 URL。
type NormalizedComment struct {
	Raw          string
	Normalized   string
	Compact      string
	Confusable   string
	DecodedTexts []string
}

// Views 从 n（归一化评论）组装所有可检测文本视图，返回去除无效重复后的视图列表。
func (n NormalizedComment) Views() []string {
	views := []string{n.Normalized, n.Compact}
	if n.Confusable != "" && n.Confusable != n.Compact {
		views = append(views, n.Confusable)
	}
	views = append(views, n.DecodedTexts...)
	return views
}

// BehaviorState 保存发布前查询到的用户、IP、重复和近重复行为计数。
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

// BehaviorMetricTrace 记录单个行为指标的观测值、预期值、窗口、阈值和跳过原因。
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

// BehaviorTrace 描述行为检测的整体执行状态及各指标明细。
type BehaviorTrace struct {
	Status            string                `json:"status"`
	ReadOnly          bool                  `json:"readOnly"`
	ContextProvided   bool                  `json:"contextProvided"`
	UnavailableReason string                `json:"unavailableReason,omitempty"`
	Metrics           []BehaviorMetricTrace `json:"metrics,omitempty"`
}

// BehaviorEvaluation 同时返回行为风险信号和可用于调试的行为轨迹。
type BehaviorEvaluation struct {
	Signals []Signal
	Trace   BehaviorTrace
}

// BehaviorKeys 集中保存一次评论对应的用户、IP、精确重复和近重复 Redis Key。
type BehaviorKeys struct {
	User          string
	IP            string
	Duplicate     string
	NearDuplicate string
}
