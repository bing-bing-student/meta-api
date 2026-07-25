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

type AdminGetCommentListRequest struct {
	Page            int    `form:"page" binding:"required,gte=1"`
	PageSize        int    `form:"pageSize" binding:"required,gte=1,lte=50"`
	ArticleID       string `form:"articleID" binding:"omitempty,lte=19"`
	ArticleTitle    string `form:"articleTitle" binding:"omitempty,lte=100"`
	AuthorHandle    string `form:"authorHandle" binding:"omitempty,lte=32"`
	CreateStartTime string `form:"createStartTime" binding:"omitempty,lte=19"`
	CreateEndTime   string `form:"createEndTime" binding:"omitempty,lte=19"`
	Status          string `form:"status" binding:"omitempty,oneof=pending approved rejected"`
}

type AdminCommentItem struct {
	ID                  string `json:"id"`
	ArticleID           string `json:"articleID"`
	ArticleTitle        string `json:"articleTitle"`
	ParentID            string `json:"parentID,omitempty"`
	UserID              string `json:"userID,omitempty"`
	ReplyToUserID       string `json:"replyToUserID,omitempty"`
	ReplyToAuthorName   string `json:"replyToAuthorName,omitempty"`
	ReplyToAuthorHandle string `json:"replyToAuthorHandle,omitempty"`
	AuthorName          string `json:"authorName"`
	AuthorHandle        string `json:"authorHandle,omitempty"`
	AvatarURL           string `json:"avatarURL,omitempty"`
	Provider            string `json:"provider,omitempty"`
	Content             string `json:"content"`
	Status              string `json:"status"`
	IP                  string `json:"ip,omitempty"`
	CreateTime          string `json:"createTime"`
	UpdateTime          string `json:"updateTime"`
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
