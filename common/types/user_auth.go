package types

import "github.com/golang-jwt/jwt/v5"

type PublicUserClaims struct {
	UserID         string `json:"userID"`
	Provider       string `json:"provider"`
	DisplayName    string `json:"displayName"`
	Handle         string `json:"handle"`
	SessionVersion int64  `json:"sessionVersion"`
	TokenUse       string `json:"tokenUse"`
	jwt.RegisteredClaims
}

type PublicUserInfo struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	DisplayName    string `json:"displayName"`
	Handle         string `json:"handle"`
	SessionVersion int64  `json:"-"`
	AvatarURL      string `json:"avatarURL,omitempty"`
	ProfileURL     string `json:"profileURL,omitempty"`
}

type OAuthProviderURIRequest struct {
	Provider string `uri:"provider" binding:"required,oneof=github google"`
}

type OAuthLoginQueryRequest struct {
	Redirect string `form:"redirect" binding:"omitempty,max=500"`
}

type OAuthLoginRequest struct {
	Provider string `uri:"provider" binding:"required,oneof=github google"`
	Redirect string `form:"redirect" binding:"omitempty,max=500"`
}

type OAuthCallbackQueryRequest struct {
	Code  string `form:"code" binding:"required"`
	State string `form:"state" binding:"required,len=64"`
}

type OAuthCallbackRequest struct {
	Provider    string `uri:"provider" binding:"required,oneof=github google"`
	Code        string `form:"code" binding:"required"`
	State       string `form:"state" binding:"required,len=64"`
	FlowBinding string `json:"-" form:"-"`
}

type OAuthCallbackResponse struct {
	User         PublicUserInfo `json:"user"`
	RedirectPath string         `json:"redirectPath"`
}
