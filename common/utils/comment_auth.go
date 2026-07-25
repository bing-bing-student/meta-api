package utils

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"meta-api/common/types"
)

const (
	CommentAccessTokenCookie = "comment_access_token"
	commentAuthCookieMaxAge  = 7 * 24 * 60 * 60
	commentAuthCookiePath    = "/"
	commentUserTokenUse      = "comment_user"
)

func GenerateCommentUserToken(user *types.PublicUserInfo) (string, error) {
	now := time.Now()
	claims := &types.PublicUserClaims{
		UserID:         user.ID,
		Provider:       user.Provider,
		DisplayName:    user.DisplayName,
		Handle:         user.Handle,
		SessionVersion: user.SessionVersion,
		TokenUse:       commentUserTokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SIGNING_KEY")))
	if err != nil {
		return "", fmt.Errorf("failed to generate comment user token: %w", err)
	}
	return tokenString, nil
}

func ParseCommentUserToken(tokenString string) (*types.PublicUserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &types.PublicUserClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
			}
			return []byte(os.Getenv("JWT_SIGNING_KEY")), nil
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("TokenExpired")
		}
		return nil, fmt.Errorf("failed to parse comment user token: %w", err)
	}
	if token == nil {
		return nil, errors.New("token is null")
	}

	claims, ok := token.Claims.(*types.PublicUserClaims)
	if !ok {
		return nil, fmt.Errorf("token claims are of incorrect type: %T", token.Claims)
	}
	if claims.TokenUse != commentUserTokenUse {
		return nil, errors.New("invalid token use")
	}
	return claims, nil
}

func SetCommentAuthCookie(c *gin.Context, token string) {
	secure := isProd()
	c.SetSameSite(commentAuthSameSiteMode())
	c.SetCookie(CommentAccessTokenCookie, token, commentAuthCookieMaxAge, commentAuthCookiePath, "", secure, true)
}

func ClearCommentAuthCookie(c *gin.Context) {
	secure := isProd()
	c.SetSameSite(commentAuthSameSiteMode())
	c.SetCookie(CommentAccessTokenCookie, "", -1, commentAuthCookiePath, "", secure, true)
}

func commentAuthSameSiteMode() http.SameSite {
	if isProd() {
		return http.SameSiteLaxMode
	}
	return http.SameSiteLaxMode
}
