package types

type GetLinkListResponse struct {
	Rows  []GetLinkListItem `json:"rows"`
	Total int               `json:"total"`
}

type GetLinkListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type AddLinkRequest struct {
	Name string `json:"name" binding:"required,lte=20"`
	URL  string `json:"url" binding:"required,lte=100"`
}

type UpdateLinkRequest struct {
	ID   string `json:"id" binding:"required,lte=19"`
	Name string `json:"name" binding:"required,lte=20"`
	URL  string `json:"url" binding:"required,lte=100"`
}

type DeleteLinkRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}
