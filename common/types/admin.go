package types

import (
	"github.com/golang-jwt/jwt/v5"
)

type UserClaims struct {
	UserID    string `json:"userID"`
	TokenUse  string `json:"tokenUse,omitempty"`
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type TokenDetails struct {
	AccessToken  string
	RefreshToken string
	AccessUUID   string
	RefreshUUID  string
	AtExpires    int64
	RtExpires    int64
}

// AccountLoginRequest 账号密码登录请求
type AccountLoginRequest struct {
	Username string `form:"username" binding:"required,max=16"`
	Password string `form:"password" binding:"required,max=16"`
	ClientIP string `json:"-" form:"-"`
}

type AccountLoginResponse struct {
	LoginChallenge string `json:"loginChallenge"`
	QRCodeURL      string `json:"qrCodeURL,omitempty"`
}

// SendSMSCodeRequest 获取短信验证码请求
type SendSMSCodeRequest struct {
	Phone string `form:"phone" binding:"required"`
}

// SMSCodeLoginRequest 短信登录请求
type SMSCodeLoginRequest struct {
	Phone string `form:"phone" binding:"required,len=11"`
	Code  string `form:"code" binding:"required,len=6"`
}

type SMSCodeLoginResponse struct {
	UserID       string `json:"userID"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// BindDynamicCodeRequest 绑定动态码请求
type BindDynamicCodeRequest struct {
	LoginChallenge string `json:"loginChallenge" form:"loginChallenge" binding:"required,len=64"`
	Code           string `json:"code" form:"code" binding:"required,len=6,numeric"`
	ClientIP       string `json:"-" form:"-"`
}

type BindDynamicCodeResponse struct {
	UserID string `json:"userID"`
	// token 改由 Set-Cookie 下发，不再序列化进响应体 data
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
}

// VerifyDynamicCodeRequest 验证动态码请求
type VerifyDynamicCodeRequest struct {
	LoginChallenge string `json:"loginChallenge" form:"loginChallenge" binding:"required,len=64"`
	Code           string `json:"code" form:"code" binding:"required,len=6,numeric"`
	ClientIP       string `json:"-" form:"-"`
}

type VerifyDynamicCodeResponse struct {
	UserID string `json:"userID"`
	// token 改由 Set-Cookie 下发，不再序列化进响应体 data
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
}

// UpdateAboutMeRequest 修改关于我请求
type UpdateAboutMeRequest struct {
	UserID          string   `json:"userID,omitempty" binding:"omitempty,lte=19"`
	Name            string   `json:"name"`
	Job             string   `json:"job"`
	WorkLife        string   `json:"workLife"`
	Address         string   `json:"address"`
	DomainInfo      string   `json:"domainInfo"`
	BlogContent     string   `json:"blogContent"`
	WebsiteLocation string   `json:"websiteLocation"`
	Statement       string   `json:"statement"`
	Email           []string `json:"email"`
}

// GetAboutMeResponse 获取关于我信息响应
type GetAboutMeResponse struct {
	Name            string   `json:"name"`
	Job             string   `json:"job"`
	WorkLife        string   `json:"workLife"`
	Address         string   `json:"address"`
	DomainInfo      string   `json:"domainInfo"`
	BlogContent     string   `json:"blogContent"`
	WebsiteLocation string   `json:"websiteLocation"`
	Statement       string   `json:"statement"`
	Email           []string `json:"email"`
}

type AdminGetUserListRequest struct {
	Page              int    `form:"page" binding:"required,gte=1"`
	PageSize          int    `form:"pageSize" binding:"required,gte=1,lte=50"`
	Handle            string `form:"handle" binding:"omitempty,lte=32"`
	DisplayName       string `form:"displayName" binding:"omitempty,lte=80"`
	Provider          string `form:"provider" binding:"omitempty,oneof=github google"`
	CommentPermission string `form:"commentPermission" binding:"omitempty,oneof=normal disabled"`
}

type AdminUserItem struct {
	ID                    string `json:"id"`
	Provider              string `json:"provider"`
	ProviderUserID        string `json:"providerUserID,omitempty"`
	DisplayName           string `json:"displayName"`
	Handle                string `json:"handle"`
	AvatarURL             string `json:"avatarURL,omitempty"`
	ProfileURL            string `json:"profileURL,omitempty"`
	Email                 string `json:"email,omitempty"`
	CommentDisabled       bool   `json:"commentDisabled"`
	CommentDisabledReason string `json:"commentDisabledReason,omitempty"`
	CommentDisabledUntil  string `json:"commentDisabledUntil,omitempty"`
	SessionVersion        int64  `json:"sessionVersion"`
	CommentCount          int64  `json:"commentCount"`
	LastCommentTime       string `json:"lastCommentTime,omitempty"`
	CreateTime            string `json:"createTime"`
	UpdateTime            string `json:"updateTime"`
}

type AdminGetUserListResponse struct {
	Rows  []AdminUserItem `json:"rows"`
	Total int             `json:"total"`
}

type AdminUpdateUserCommentPermissionRequest struct {
	ID            string `json:"id" binding:"required,lte=19"`
	Disabled      bool   `json:"disabled"`
	Reason        string `json:"reason" binding:"omitempty,lte=200"`
	DisabledUntil string `json:"disabledUntil" binding:"omitempty,lte=19"`
}

type AdminForceUserLogoutRequest struct {
	ID string `json:"id" binding:"required,lte=19"`
}
