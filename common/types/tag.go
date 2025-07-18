package types

type AdminGetTagListResponse struct {
}

type AdminGetArticleListByTagRequest struct {
	TagName  string `form:"tagName" binding:"required,lte=20"`
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=9"`
}

type AdminGetArticleListByTagItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	ViewNum    int    `json:"viewNum"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

type AdminGetArticleListByTagResponse struct {
	Rows  []AdminGetArticleListByTagItem `json:"rows"`
	Total int                            `json:"total"`
}

type AdminUpdateTagRequest struct {
	ArticleIDList []string `json:"articleIDList" binding:"required,articleID"`
	NewTagName    string   `json:"newTagName" binding:"required,lte=20"`
	OldTagName    string   `json:"oldTagName" binding:"required,lte=20"`
}

type UserGetTagListItem struct {
	Name       string `json:"name"`
	ArticleNum int    `json:"articleNum"`
}

type UserGetTagListResponse struct {
	Rows  []UserGetTagListItem `json:"rows"`
	Total int                  `json:"total"`
}

type UserGetArticleListByTagRequest struct {
	TagName  string `form:"tagName" binding:"required,lte=20"`
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=9"`
}

type UserGetArticleListByTagResponse struct {
	Rows  []UserGetArticleItem `json:"rows"`
	Total int                  `json:"total"`
}
