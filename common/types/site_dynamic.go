package types

type AdminSiteDynamicItem struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	SortOrder  int    `json:"sortOrder"`
	EventDate  string `json:"eventDate"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

type UserSiteDynamicItem struct {
	Content   string `json:"content"`
	EventDate string `json:"eventDate"`
}

type AdminGetSiteDynamicListResponse struct {
	Rows  []AdminSiteDynamicItem `json:"rows"`
	Total int                    `json:"total"`
}

type AdminAddSiteDynamicRequest struct {
	Content   string `json:"content" binding:"required,lte=50"`
	Status    string `json:"status" binding:"required,oneof=published hidden"`
	EventDate string `json:"eventDate" binding:"required,len=10"`
}

type AdminUpdateSiteDynamicRequest struct {
	ID        string `json:"id" binding:"required,lte=19"`
	Content   string `json:"content" binding:"required,lte=50"`
	Status    string `json:"status" binding:"required,oneof=published hidden"`
	EventDate string `json:"eventDate" binding:"required,len=10"`
}

type AdminReorderSiteDynamicRequest struct {
	IDs []string `json:"ids" binding:"required,min=1,dive,required,lte=19"`
}

type AdminDeleteSiteDynamicRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}

type UserGetSiteDynamicListResponse struct {
	Rows  []UserSiteDynamicItem `json:"rows"`
	Total int                   `json:"total"`
}
