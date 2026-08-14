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
	Title    string `json:"title" binding:"required,max=100"`
	Tag      string `json:"tag" binding:"required,max=20"`
	Describe string `json:"describe" binding:"required,max=200"`
	Content  string `json:"content" binding:"required"`
}

// AdminSaveArticleResponse 返回新增或修改后的文章 ID。
type AdminSaveArticleResponse struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type AdminUpdateArticleRequest struct {
	ID       string `json:"id" binding:"required,lte=19"`
	Title    string `json:"title" binding:"required,max=100"`
	Tag      string `json:"tag" binding:"required,max=20"`
	Describe string `json:"describe" binding:"required,max=200"`
	Content  string `json:"content" binding:"required"`
}

type AdminDeleteArticleRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}

type AdminGetArticleDraftListRequest struct {
	Page     int `form:"page" binding:"required,gte=1"`
	PageSize int `form:"pageSize" binding:"required,gte=1,lte=10"`
}

type AdminArticleDraftListItem struct {
	ID         string `json:"id"`
	ArticleID  string `json:"articleID,omitempty"`
	DraftType  string `json:"draftType"`
	Title      string `json:"title"`
	Tag        string `json:"tag"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

type AdminGetArticleDraftListResponse struct {
	Rows  []AdminArticleDraftListItem `json:"rows"`
	Total int                         `json:"total"`
}

type AdminGetArticleDraftDetailRequest struct {
	ID string `form:"id" binding:"required,lte=19"`
}

type AdminGetArticleDraftDetailResponse struct {
	ID        string `json:"id"`
	ArticleID string `json:"articleID,omitempty"`
	Title     string `json:"title"`
	Tag       string `json:"tag"`
	Describe  string `json:"describe"`
	Content   string `json:"content"`
}

type AdminSaveArticleDraftRequest struct {
	ID        string `json:"id" binding:"omitempty,lte=19"`
	ArticleID string `json:"articleID" binding:"omitempty,lte=19"`
	Title     string `json:"title" binding:"omitempty,max=100"`
	Tag       string `json:"tag" binding:"omitempty,max=20"`
	Describe  string `json:"describe" binding:"omitempty,max=200"`
	Content   string `json:"content"`
}

type AdminPublishArticleDraftRequest struct {
	ID       string `json:"id" binding:"required,lte=19"`
	Title    string `json:"title" binding:"required,max=100"`
	Tag      string `json:"tag" binding:"required,max=20"`
	Describe string `json:"describe" binding:"required,max=200"`
	Content  string `json:"content" binding:"required"`
}

type AdminDeleteArticleDraftRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}

type AdminUploadArticleImageResponse struct {
	URL       string `json:"url"`
	ImageName string `json:"imageName"`
	Size      int64  `json:"size"`
	Mime      string `json:"mime"`
}

type AdminScanArticleImagesResponse struct {
	ArticleTotal   int `json:"articleTotal"`
	ImageTotal     int `json:"imageTotal"`
	ReferenceTotal int `json:"referenceTotal"`
	UsedTotal      int `json:"usedTotal"`
	UnusedTotal    int `json:"unusedTotal"`
	MissingTotal   int `json:"missingTotal"`
}

type AdminGetArticleImageListRequest struct {
	Page     int    `form:"page" binding:"required,gte=1"`
	PageSize int    `form:"pageSize" binding:"required,gte=1,lte=20"`
	Status   string `form:"status" binding:"omitempty,oneof=used unused missing"`
	Keyword  string `form:"keyword" binding:"omitempty,lte=120"`
}

type AdminArticleImageListItem struct {
	ID                 string `json:"id"`
	URL                string `json:"url"`
	ImageName          string `json:"imageName"`
	Mime               string `json:"mime"`
	Size               int64  `json:"size"`
	ETag               string `json:"etag"`
	Status             string `json:"status"`
	Source             string `json:"source"`
	RefCount           int    `json:"refCount"`
	LastSeenTime       string `json:"lastSeenTime"`
	ObjectModifiedTime string `json:"objectModifiedTime"`
	CreateTime         string `json:"createTime"`
	UpdateTime         string `json:"updateTime"`
}

type AdminGetArticleImageListResponse struct {
	Rows  []AdminArticleImageListItem `json:"rows"`
	Total int                         `json:"total"`
}

type AdminGetArticleImageDetailRequest struct {
	ID string `form:"id" binding:"required,lte=19"`
}

type AdminArticleImageReferenceItem struct {
	ID           string `json:"id"`
	ArticleID    string `json:"articleID"`
	ArticleTitle string `json:"articleTitle"`
}

type AdminGetArticleImageDetailResponse struct {
	Image      AdminArticleImageListItem        `json:"image"`
	References []AdminArticleImageReferenceItem `json:"references"`
}

type AdminDeleteArticleImageRequest struct {
	ID    string `json:"id" binding:"required,lte=19"`
	Force bool   `json:"force"`
}

type UserGetArticleListRequest struct {
	Page     int `form:"page" binding:"required,gte=1"`
	PageSize int `form:"pageSize" binding:"required,gte=1,lte=10"`
}

type UserGetArticleItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TagName    string `json:"tagName,omitempty"`
	Describe   string `json:"describe,omitempty"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime,omitempty"`
	ViewNum    int    `json:"viewNum"`
}

type UserGetArticleListResponse struct {
	Rows  []UserGetArticleItem `json:"rows"`
	Total int                  `json:"total"`
}

type UserSearchArticleRequest struct {
	Word     string `json:"word" form:"word" binding:"required,lte=30"`
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
	ID string `form:"id" binding:"required,lte=19"`
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
