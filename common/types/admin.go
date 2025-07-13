package types

import (
	"github.com/golang-jwt/jwt/v5"
)

type UserClaims struct {
	UserID string
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

// AccountRegisterRequest 账号密码注册请求
type AccountRegisterRequest struct {
	Username string `form:"username" binding:"required"`
	Password string `form:"password" binding:"required"`
	Phone    string `form:"phone" binding:"required"`
}

// AccountLoginRequest 账号密码登录请求
type AccountLoginRequest struct {
	Username string `form:"username" binding:"required,max=16"`
	Password string `form:"password" binding:"required,max=16"`
}

type AccountLoginResponse struct {
	UserID    string `json:"userID"`
	QRCodeURL string `json:"qrCodeURL,omitempty"`
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
	UserID string `form:"userID" binding:"required,lte=19"`
	Code   string `form:"code" binding:"required"`
}

type BindDynamicCodeResponse struct {
	UserID       string `json:"userID"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// VerifyDynamicCodeRequest 验证动态码请求
type VerifyDynamicCodeRequest struct {
	UserID string `form:"userID" binding:"required,lte=19"`
	Code   string `form:"code" binding:"required"`
}

type VerifyDynamicCodeResponse struct {
	UserID       string `json:"userID"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// UpdateAboutMeRequest 修改关于我请求
type UpdateAboutMeRequest struct {
	ID              string   `json:"id" binding:"required,lte=19"`
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
