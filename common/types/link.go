package types

type LinkItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type AdminGetLinkListResponse struct {
	Rows  []LinkItem `json:"rows"`
	Total int        `json:"total"`
}

type AdminAddLinkRequest struct {
	Name string `json:"name" binding:"required,lte=20"`
	URL  string `json:"url" binding:"required,lte=100"`
}

type AdminUpdateLinkRequest struct {
	ID   string `json:"id" binding:"required,lte=19"`
	Name string `json:"name" binding:"required,lte=20"`
	URL  string `json:"url" binding:"required,lte=100"`
}

type AdminDeleteLinkRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}

type UserGetLinkListResponse struct {
	Rows  []LinkItem `json:"rows"`
	Total int        `json:"total"`
}
