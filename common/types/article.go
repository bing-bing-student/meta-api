package types

type AdminGetArticleListRequest struct {
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=10"`
	Order    string `form:"order" binding:"required,oneof=time view"`
}

type AdminGetArticleListItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Tag        string `json:"tag"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
	ViewNum    int    `json:"viewNum"`
}

type AdminGetArticleListResponse struct {
	Rows  []AdminGetArticleListItem `json:"rows"`
	Total int                       `json:"total"`
}

type AdminGetArticleDetailRequest struct {
	ID string `form:"id" binding:"required,lte=19"`
}

type AdminGetArticleDetailResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Tag      string `json:"tag"`
	Describe string `json:"describe"`
	Content  string `json:"content"`
}

type AdminAddArticleRequest struct {
	Title    string `json:"title" binding:"required,max=30"`
	Tag      string `json:"tag" binding:"required,max=20"`
	Describe string `json:"describe" binding:"required,max=200"`
	Content  string `json:"content" binding:"required"`
}

type AdminUpdateArticleRequest struct {
	ID       string `json:"id" binding:"required,lte=19"`
	Title    string `json:"title" binding:"required,max=30"`
	Tag      string `json:"tag" binding:"required,max=20"`
	Describe string `json:"describe" binding:"required,max=200"`
	Content  string `json:"content" binding:"required"`
}

type AdminDeleteArticleRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}

type UserGetArticleListRequest struct {
	Page     int `form:"page" binding:"required,gte=1"`
	PageSize int `form:"pageSize" binding:"required,gte=1,lte=10"`
}

type UserGetArticleItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TagName    string `json:"tagName"`
	Describe   string `json:"describe"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
	ViewNum    int    `json:"viewNum"`
}

type UserGetArticleListResponse struct {
	Rows  []UserGetArticleItem `json:"rows"`
	Total int                  `json:"total"`
}

type UserSearchArticleRequest struct {
	Word     string `json:"word" form:"word" binding:"required,lte=20"`
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=10"`
}

type UserSearchArticleResponse struct {
	Rows  []UserGetArticleItem `json:"rows"`
	Total int                  `json:"total"`
}

type GetHotArticleItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	ViewNum int    `json:"viewNum"`
}

type UserGetHotArticleResponse struct {
	Rows  []GetHotArticleItem `json:"rows"`
	Total int                 `json:"total"`
}

type UserGetArticleDetailRequest struct {
	UserID string `json:"-"`
	ID     string `form:"id" binding:"required,lte=19"`
}

type UserGetArticleDetailResponse struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TagName    string `json:"tag"`
	Content    string `json:"content"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

type GetTimelineListItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	CreateTime string `json:"createTime"`
}

type GetTimelineRowsItem struct {
	Time string                `json:"time"`
	List []GetTimelineListItem `json:"list"`
}

type GetTimelineResponse struct {
	Rows  []GetTimelineRowsItem `json:"rows"`
	Total int                   `json:"total"`
}
