package utils

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const (
	// AccessTokenCookie access_token 的 Cookie 名
	AccessTokenCookie = "access_token"
	// RefreshTokenCookie refresh_token 的 Cookie 名
	RefreshTokenCookie = "refresh_token"

	// accessCookieMaxAge access_token Cookie 有效期，与 access JWT TTL 保持一致（15 分钟）
	accessCookieMaxAge = 15 * 60
	// refreshCookieMaxAge refresh_token Cookie 有效期，与 refresh JWT TTL 保持一致（7 天）
	refreshCookieMaxAge = 7 * 24 * 60 * 60

	// accessCookiePath access_token 在所有路径下携带
	accessCookiePath = "/"
	// refreshCookiePath refresh_token 仅在刷新接口携带，Path 为浏览器实际请求路径（含 /api 前缀）
	refreshCookiePath = "/api/admin/refresh-token"
)

// isProd 控制 Cookie 的 Secure 属性：生产环境（HTTPS）下必须为 true，
// 本地 HTTP 调试时为 false，否则浏览器不会保存 Secure Cookie。
// 通过环境变量 APP_ENV=production 开启。
func isProd() bool {
	return os.Getenv("APP_ENV") == "production"
}

// SetAuthCookies 登录/刷新成功时下发 access_token 和 refresh_token 两个 HttpOnly Cookie
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string) {
	secure := isProd()
	sameSite := sameSiteMode()
	// gin 的 SetCookie 第 6 个参数 secure、第 7 个 httpOnly；SameSite 需用 SetSameSite 设置
	c.SetSameSite(sameSite)
	c.SetCookie(AccessTokenCookie, accessToken, accessCookieMaxAge, accessCookiePath, "", secure, true)
	c.SetSameSite(sameSite)
	c.SetCookie(RefreshTokenCookie, refreshToken, refreshCookieMaxAge, refreshCookiePath, "", secure, true)
}

// ClearAuthCookies 登出/刷新失败时清除两个 Cookie，Path 必须与下发时一致，否则无法删除
func ClearAuthCookies(c *gin.Context) {
	secure := isProd()
	sameSite := sameSiteMode()
	c.SetSameSite(sameSite)
	c.SetCookie(AccessTokenCookie, "", -1, accessCookiePath, "", secure, true)
	c.SetSameSite(sameSite)
	c.SetCookie(RefreshTokenCookie, "", -1, refreshCookiePath, "", secure, true)
}

// sameSiteMode 生产用 Strict（最强 CSRF 防护），本地 HTTP 调试用 Lax 兜底，
// 避免在跨端口反代、HTTP localhost 等边界场景下 Cookie 被浏览器拦截。
func sameSiteMode() http.SameSite {
	if isProd() {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}
