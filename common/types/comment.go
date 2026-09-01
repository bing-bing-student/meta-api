package types

type UserGetCommentListRequest struct {
	ArticleID string `form:"articleID" binding:"required,lte=19"`
	Page      int    `form:"page" binding:"required,gte=1"`
	PageSize  int    `form:"pageSize" binding:"required,gte=1,lte=50"`
}

type UserGetCommentReplyListRequest struct {
	ParentID string `form:"parentID" binding:"required,lte=19"`
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=50"`
}

type UserCommentItem struct {
	ID                    string            `json:"id"`
	ArticleID             string            `json:"articleID"`
	ParentID              string            `json:"parentID,omitempty"`
	UserID                string            `json:"userID"`
	ReplyToUserID         string            `json:"replyToUserID,omitempty"`
	ReplyToCommentID      string            `json:"replyToCommentID,omitempty"`
	ReplyToAuthorName     string            `json:"replyToAuthorName,omitempty"`
	ReplyToAuthorHandle   string            `json:"replyToAuthorHandle,omitempty"`
	ReplyToContentExcerpt string            `json:"replyToContentExcerpt,omitempty"`
	AuthorName            string            `json:"authorName"`
	AuthorHandle          string            `json:"authorHandle,omitempty"`
	AvatarURL             string            `json:"avatarURL,omitempty"`
	Content               string            `json:"content"`
	CreateTime            string            `json:"createTime"`
	Replies               []UserCommentItem `json:"replies,omitempty"`
	ReplyHasMore          bool              `json:"replyHasMore,omitempty"`
	ReplyNextPage         int               `json:"replyNextPage,omitempty"`
}

type UserGetCommentListResponse struct {
	Rows  []UserCommentItem `json:"rows"`
	Total int               `json:"total"`
}

type UserGetCommentReplyListResponse struct {
	Rows     []UserCommentItem `json:"rows"`
	Total    int               `json:"total"`
	HasMore  bool              `json:"hasMore"`
	NextPage int               `json:"nextPage,omitempty"`
}

type UserAddCommentRequest struct {
	ArticleID        string `json:"articleID" form:"articleID" binding:"required,lte=19"`
	ParentID         string `json:"parentID" form:"parentID" binding:"omitempty,lte=19"`
	ReplyToCommentID string `json:"replyToCommentID" form:"replyToCommentID" binding:"omitempty,lte=19"`
	Content          string `json:"content" form:"content" binding:"required,min=1,max=1000"`
	UserID           string `json:"-" form:"-"`
	SessionVersion   int64  `json:"-" form:"-"`
	ClientIP         string `json:"-" form:"-"`
}

type UserAddCommentResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type UserReportCommentRequest struct {
	CommentID      string `json:"commentID" form:"commentID" binding:"required,lte=19"`
	Reason         string `json:"reason" form:"reason" binding:"omitempty,lte=200"`
	UserID         string `json:"-" form:"-"`
	SessionVersion int64  `json:"-" form:"-"`
	ClientIP       string `json:"-" form:"-"`
}

type UserReportCommentResponse struct {
	CommentID   string `json:"commentID"`
	ReportCount int64  `json:"reportCount"`
	Status      string `json:"status"`
}

type UserGetCommentReportStatusRequest struct {
	CommentIDs     []string `json:"commentIDs" form:"commentIDs" binding:"required,min=1,max=100,dive,lte=19"`
	UserID         string   `json:"-" form:"-"`
	SessionVersion int64    `json:"-" form:"-"`
}

type UserGetCommentReportStatusResponse struct {
	ReportedCommentIDs []string `json:"reportedCommentIDs"`
}

type AdminGetCommentListRequest struct {
	Page            int    `form:"page" binding:"required,gte=1"`
	PageSize        int    `form:"pageSize" binding:"required,gte=1,lte=50"`
	ArticleID       string `form:"articleID" binding:"omitempty,lte=19"`
	ArticleTitle    string `form:"articleTitle" binding:"omitempty,lte=100"`
	ContentKeyword  string `form:"contentKeyword" binding:"omitempty,lte=100"`
	AuthorHandle    string `form:"authorHandle" binding:"omitempty,lte=32"`
	CreateStartTime string `form:"createStartTime" binding:"omitempty,lte=19"`
	CreateEndTime   string `form:"createEndTime" binding:"omitempty,lte=19"`
	Status          string `form:"status" binding:"omitempty,oneof=pending approved rejected"`
}

type AdminCommentItem struct {
	ID                  string   `json:"id"`
	ArticleTitle        string   `json:"articleTitle"`
	ParentID            string   `json:"parentID,omitempty"`
	ReplyToAuthorName   string   `json:"replyToAuthorName,omitempty"`
	ReplyToAuthorHandle string   `json:"replyToAuthorHandle,omitempty"`
	AuthorHandle        string   `json:"authorHandle,omitempty"`
	Content             string   `json:"content"`
	Status              string   `json:"status"`
	ModerationReasons   []string `json:"moderationReasons,omitempty"`
	IP                  string   `json:"ip,omitempty"`
	CreateTime          string   `json:"createTime"`
	UpdateTime          string   `json:"updateTime"`
}

type AdminGetCommentListResponse struct {
	Rows  []AdminCommentItem `json:"rows"`
	Total int                `json:"total"`
}

type AdminGetCommentDetailRequest struct {
	ID string `form:"id" binding:"required,lte=19"`
}

type AdminCommentModerationAuditDetail struct {
	AuditID       string                                 `json:"auditID"`
	Source        string                                 `json:"source"`
	PolicyVersion string                                 `json:"policyVersion"`
	EvaluatedAt   string                                 `json:"evaluatedAt"`
	Context       AdminCommentModerationExecutionContext `json:"context"`
	Result        AdminPreviewCommentModerationItem      `json:"result"`
}

type AdminGetCommentDetailResponse struct {
	Comment            AdminCommentItem                   `json:"comment"`
	Moderation         *AdminCommentModerationAuditDetail `json:"moderation,omitempty"`
	FeedbackCategories []string                           `json:"feedbackCategories"`
}

type AdminUpdateCommentStatusRequest struct {
	ID     string `json:"id" binding:"required,lte=19"`
	Status string `json:"status" binding:"required,oneof=pending approved rejected"`
}

type AdminReviewCommentRequest struct {
	AdminID          string `json:"-"`
	ID               string `json:"id" binding:"required,lte=19"`
	Status           string `json:"status" binding:"required,oneof=pending approved rejected"`
	IncludeFeedback  bool   `json:"includeFeedback"`
	AuditID          string `json:"auditID" binding:"omitempty,lte=20"`
	ExpectedCategory string `json:"expectedCategory" binding:"omitempty,max=64"`
	Note             string `json:"note" binding:"omitempty,max=500"`
}

type AdminDeleteCommentRequest struct {
	ID     string   `json:"id" binding:"omitempty,lte=19"`
	IDList []string `json:"idList" binding:"omitempty,dive,lte=19"`
}

type AdminPreviewCommentModerationRequest struct {
	AdminID         string   `json:"-"`
	Content         string   `json:"content" binding:"omitempty,max=1000"`
	Comments        []string `json:"comments" binding:"omitempty,max=5000,dive,max=1000"`
	UserID          string   `json:"userID" binding:"omitempty,lte=19"`
	ArticleID       string   `json:"articleID" binding:"omitempty,lte=19"`
	ClientIP        string   `json:"clientIP" binding:"omitempty,lte=64"`
	ArticleTitle    string   `json:"articleTitle" binding:"omitempty,max=100"`
	ArticleCategory string   `json:"articleCategory" binding:"omitempty,max=100"`
	ParentContent   string   `json:"parentContent" binding:"omitempty,max=1000"`
	ReplyToContent  string   `json:"replyToContent" binding:"omitempty,max=1000"`
}

type AdminCommentModerationTextView struct {
	Raw          string   `json:"raw"`
	Normalized   string   `json:"normalized"`
	Compact      string   `json:"compact"`
	Confusable   string   `json:"confusable,omitempty"`
	DecodedTexts []string `json:"decodedTexts,omitempty"`
}

type AdminCommentModerationSignal struct {
	Source     string `json:"source"`
	Category   string `json:"category,omitempty"`
	Level      string `json:"level,omitempty"`
	Score      int    `json:"score"`
	Reason     string `json:"reason,omitempty"`
	ReasonText string `json:"reasonText,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	RuleID     string `json:"ruleID,omitempty"`
	ClauseID   int    `json:"clauseID,omitempty"`
}

type AdminCommentModerationClause struct {
	ID   int                            `json:"id"`
	Text AdminCommentModerationTextView `json:"text"`
}

type AdminCommentModerationTrace struct {
	Clauses           []AdminCommentModerationClause             `json:"clauses"`
	DetectorSignals   []AdminCommentModerationSignal             `json:"detectorSignals,omitempty"`
	SuppressedSignals []AdminCommentModerationSignal             `json:"suppressedSignals,omitempty"`
	Behavior          AdminCommentModerationBehaviorTrace        `json:"behavior"`
	DecisionEngine    *AdminCommentModerationDecisionEngineTrace `json:"decisionEngine,omitempty"`
	Decisions         AdminCommentModerationDecisionFlowTrace    `json:"decisions"`
}

type AdminCommentModerationBehaviorMetric struct {
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

type AdminCommentModerationBehaviorTrace struct {
	Status            string                                 `json:"status"`
	ReadOnly          bool                                   `json:"readOnly"`
	ContextProvided   bool                                   `json:"contextProvided"`
	UnavailableReason string                                 `json:"unavailableReason,omitempty"`
	Metrics           []AdminCommentModerationBehaviorMetric `json:"metrics,omitempty"`
}

type AdminCommentModerationRewriteCandidate struct {
	Text       string  `json:"text"`
	Observed   string  `json:"observed,omitempty"`
	Category   string  `json:"category,omitempty"`
	Role       string  `json:"role,omitempty"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	Ambiguous  bool    `json:"ambiguous"`
	Rationale  string  `json:"rationale,omitempty"`
	ClauseID   int     `json:"clauseID,omitempty"`
}

type AdminCommentModerationEvidence struct {
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Category         string  `json:"category"`
	Polarity         string  `json:"polarity"`
	Confidence       float64 `json:"confidence"`
	CorrelationGroup string  `json:"correlationGroup"`
	Value            string  `json:"value,omitempty"`
	RuleID           string  `json:"ruleID,omitempty"`
	ClauseID         int     `json:"clauseID,omitempty"`
}

type AdminCommentModerationContextAssessment struct {
	Analyzed          bool                             `json:"analyzed"`
	Confidence        float64                          `json:"confidence"`
	Intent            string                           `json:"intent,omitempty"`
	BenignProbability float64                          `json:"benignProbability"`
	Evidence          []AdminCommentModerationEvidence `json:"evidence,omitempty"`
	Relations         []AdminCommentModerationRelation `json:"relations,omitempty"`
	Explanation       string                           `json:"explanation,omitempty"`
	UnavailableReason string                           `json:"unavailableReason,omitempty"`
}

type AdminCommentModerationEvidenceDeduplication struct {
	Discarded AdminCommentModerationEvidence `json:"discarded"`
	KeptID    string                         `json:"keptID"`
	Reason    string                         `json:"reason"`
}

type AdminCommentModerationRelation struct {
	ID         string  `json:"id"`
	ClauseID   int     `json:"clauseID"`
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

type AdminCommentModerationProbabilityDecision struct {
	Status                string             `json:"status"`
	RiskProbability       float64            `json:"riskProbability"`
	Confidence            float64            `json:"confidence"`
	Decision              string             `json:"decision"`
	Calibration           string             `json:"calibration"`
	CategoryProbabilities map[string]float64 `json:"categoryProbabilities,omitempty"`
	Actionable            bool               `json:"actionable"`
	FallbackReason        string             `json:"fallbackReason,omitempty"`
}

type AdminCommentModerationProbabilityThresholds struct {
	ApproveMax    float64 `json:"approveMax"`
	RejectMin     float64 `json:"rejectMin"`
	MinConfidence float64 `json:"minConfidence"`
}

type AdminCommentModerationCategoryFusion struct {
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

type AdminCommentModerationEvidenceFusion struct {
	Thresholds   AdminCommentModerationProbabilityThresholds   `json:"thresholds"`
	Categories   []AdminCommentModerationCategoryFusion        `json:"categories,omitempty"`
	InputCount   int                                           `json:"inputCount"`
	OutputCount  int                                           `json:"outputCount"`
	Deduplicated []AdminCommentModerationEvidenceDeduplication `json:"deduplicated,omitempty"`
}

type AdminCommentModerationDecisionEngineTrace struct {
	Candidates []AdminCommentModerationRewriteCandidate  `json:"candidates,omitempty"`
	Evidence   []AdminCommentModerationEvidence          `json:"evidence,omitempty"`
	Context    AdminCommentModerationContextAssessment   `json:"context"`
	Fusion     AdminCommentModerationEvidenceFusion      `json:"fusion"`
	Decision   AdminCommentModerationProbabilityDecision `json:"decision"`
}

type AdminCommentModerationDecisionSnapshot struct {
	Status   string `json:"status"`
	Score    int    `json:"score"`
	Decision string `json:"decision"`
}

type AdminCommentModerationDecisionApplication struct {
	Evaluated bool                                   `json:"evaluated"`
	Applied   bool                                   `json:"applied"`
	Before    AdminCommentModerationDecisionSnapshot `json:"before"`
	Candidate AdminCommentModerationDecisionSnapshot `json:"candidate"`
	After     AdminCommentModerationDecisionSnapshot `json:"after"`
	Reason    string                                 `json:"reason,omitempty"`
}

type AdminCommentModerationHardSafety struct {
	Evaluated bool                                   `json:"evaluated"`
	Triggered bool                                   `json:"triggered"`
	RuleID    string                                 `json:"ruleID,omitempty"`
	Before    AdminCommentModerationDecisionSnapshot `json:"before"`
	After     AdminCommentModerationDecisionSnapshot `json:"after"`
	Reason    string                                 `json:"reason,omitempty"`
}

type AdminCommentModerationFeedbackApplication struct {
	Evaluated         bool                                   `json:"evaluated"`
	Matched           bool                                   `json:"matched"`
	Consensus         bool                                   `json:"consensus"`
	Applied           bool                                   `json:"applied"`
	Scope             string                                 `json:"scope,omitempty"`
	Support           int64                                  `json:"support"`
	Total             int64                                  `json:"total"`
	Conflicts         int64                                  `json:"conflicts"`
	SimulationSupport int64                                  `json:"simulationSupport"`
	LiveSupport       int64                                  `json:"liveSupport"`
	ExpectedStatus    string                                 `json:"expectedStatus,omitempty"`
	ExpectedCategory  string                                 `json:"expectedCategory,omitempty"`
	Before            AdminCommentModerationDecisionSnapshot `json:"before"`
	After             AdminCommentModerationDecisionSnapshot `json:"after"`
	Reason            string                                 `json:"reason,omitempty"`
}

type AdminCommentModerationDecisionFlowTrace struct {
	Rule        AdminCommentModerationDecisionSnapshot    `json:"rule"`
	Probability AdminCommentModerationDecisionApplication `json:"probability"`
	HardSafety  AdminCommentModerationHardSafety          `json:"hardSafety"`
	Feedback    AdminCommentModerationFeedbackApplication `json:"feedback"`
	Final       AdminCommentModerationDecisionSnapshot    `json:"final"`
}

type AdminCommentModerationExecutionContext struct {
	UserID                  string `json:"userID,omitempty"`
	ArticleID               string `json:"articleID,omitempty"`
	ClientIPProvided        bool   `json:"clientIPProvided"`
	ArticleTitleProvided    bool   `json:"articleTitleProvided"`
	ArticleCategoryProvided bool   `json:"articleCategoryProvided"`
	ParentContentProvided   bool   `json:"parentContentProvided"`
	ReplyContentProvided    bool   `json:"replyContentProvided"`
}

type AdminPreviewCommentModerationItem struct {
	AuditID    string                         `json:"auditID,omitempty"`
	Line       int                            `json:"line"`
	Content    string                         `json:"content"`
	Status     string                         `json:"status"`
	RiskScore  int                            `json:"riskScore"`
	Decision   string                         `json:"decision"`
	Reasons    []string                       `json:"reasons,omitempty"`
	RawReasons []string                       `json:"rawReasons,omitempty"`
	Signals    []AdminCommentModerationSignal `json:"signals,omitempty"`
	Text       AdminCommentModerationTextView `json:"text"`
	Trace      AdminCommentModerationTrace    `json:"trace"`
}

type AdminPreviewCommentModerationResponse struct {
	BatchID            string                                 `json:"batchID,omitempty"`
	EvaluatedAt        string                                 `json:"evaluatedAt"`
	PolicyVersion      string                                 `json:"policyVersion"`
	FeedbackCategories []string                               `json:"feedbackCategories"`
	Context            AdminCommentModerationExecutionContext `json:"context"`
	Rows               []AdminPreviewCommentModerationItem    `json:"rows"`
	Total              int                                    `json:"total"`
	Approved           int                                    `json:"approved"`
	Pending            int                                    `json:"pending"`
	Rejected           int                                    `json:"rejected"`
	BehaviorDryRun     bool                                   `json:"behaviorDryRun"`
}

type AdminCommentModerationRelationCorrection struct {
	Subject  string `json:"subject" binding:"omitempty,max=100"`
	Action   string `json:"action" binding:"omitempty,max=100"`
	Object   string `json:"object" binding:"omitempty,max=200"`
	Result   string `json:"result" binding:"omitempty,max=200"`
	Negated  *bool  `json:"negated"`
	Quoted   *bool  `json:"quoted"`
	Reported *bool  `json:"reported"`
}

type AdminSubmitCommentModerationFeedbackRequest struct {
	AdminID          string                                    `json:"-"`
	AuditID          string                                    `json:"auditID" binding:"required,lte=20"`
	ExpectedStatus   string                                    `json:"expectedStatus" binding:"required,oneof=pending approved rejected"`
	ExpectedCategory string                                    `json:"expectedCategory" binding:"omitempty,max=64"`
	Relation         *AdminCommentModerationRelationCorrection `json:"relation"`
	Note             string                                    `json:"note" binding:"omitempty,max=500"`
}

type AdminGetCommentReportListRequest struct {
	Page           int    `form:"page" binding:"required,gte=1"`
	PageSize       int    `form:"pageSize" binding:"required,gte=1,lte=50"`
	CommentQuery   string `form:"commentQuery" binding:"omitempty,lte=100"`
	AuthorHandle   string `form:"authorHandle" binding:"omitempty,lte=32"`
	ReporterHandle string `form:"reporterHandle" binding:"omitempty,lte=32"`
	Status         string `form:"status" binding:"omitempty,oneof=pending accepted rejected"`
}

type AdminCommentReportItem struct {
	ID                  string `json:"id"`
	CommentID           string `json:"commentID"`
	ArticleID           string `json:"articleID"`
	ArticleTitle        string `json:"articleTitle"`
	CommentAuthorID     string `json:"commentAuthorID,omitempty"`
	CommentAuthorName   string `json:"commentAuthorName"`
	CommentAuthorHandle string `json:"commentAuthorHandle,omitempty"`
	CommentContent      string `json:"commentContent"`
	CommentStatus       string `json:"commentStatus"`
	ReporterID          string `json:"reporterID"`
	ReporterName        string `json:"reporterName"`
	ReporterHandle      string `json:"reporterHandle,omitempty"`
	Reason              string `json:"reason,omitempty"`
	Status              string `json:"status"`
	IP                  string `json:"ip,omitempty"`
	CreateTime          string `json:"createTime"`
	UpdateTime          string `json:"updateTime"`
}

type AdminGetCommentReportListResponse struct {
	Rows  []AdminCommentReportItem `json:"rows"`
	Total int                      `json:"total"`
}

type AdminHandleCommentReportRequest struct {
	CommentID string `json:"commentID" binding:"required,lte=19"`
	Action    string `json:"action" binding:"required,oneof=accept reject"`
}
