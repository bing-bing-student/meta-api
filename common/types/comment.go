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

type AdminUpdateCommentStatusRequest struct {
	ID     string `json:"id" binding:"required,lte=19"`
	Status string `json:"status" binding:"required,oneof=pending approved rejected"`
}

type AdminDeleteCommentRequest struct {
	ID     string   `json:"id" binding:"omitempty,lte=19"`
	IDList []string `json:"idList" binding:"omitempty,dive,lte=19"`
}

type AdminPreviewCommentModerationRequest struct {
	Content   string   `json:"content" binding:"omitempty,max=1000"`
	Comments  []string `json:"comments" binding:"omitempty,max=5000,dive,max=1000"`
	UserID    string   `json:"userID" binding:"omitempty,lte=19"`
	ArticleID string   `json:"articleID" binding:"omitempty,lte=19"`
	ClientIP  string   `json:"clientIP" binding:"omitempty,lte=64"`
}

type AdminCommentModerationTextView struct {
	Raw          string   `json:"raw"`
	Normalized   string   `json:"normalized"`
	Compact      string   `json:"compact"`
	PinyinFolded string   `json:"pinyinFolded,omitempty"`
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
}

type AdminPreviewCommentModerationItem struct {
	Line           int                            `json:"line"`
	Content        string                         `json:"content"`
	Status         string                         `json:"status"`
	RiskScore      int                            `json:"riskScore"`
	FinalScore     int                            `json:"finalScore"`
	Decision       string                         `json:"decision"`
	Reasons        []string                       `json:"reasons,omitempty"`
	RawReasons     []string                       `json:"rawReasons,omitempty"`
	Signals        []AdminCommentModerationSignal `json:"signals,omitempty"`
	Text           AdminCommentModerationTextView `json:"text"`
	BehaviorDryRun bool                           `json:"behaviorDryRun"`
}

type AdminPreviewCommentModerationResponse struct {
	Rows           []AdminPreviewCommentModerationItem `json:"rows"`
	Total          int                                 `json:"total"`
	Approved       int                                 `json:"approved"`
	Pending        int                                 `json:"pending"`
	Rejected       int                                 `json:"rejected"`
	BehaviorDryRun bool                                `json:"behaviorDryRun"`
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
