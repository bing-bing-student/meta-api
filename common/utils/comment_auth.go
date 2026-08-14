package utils

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"meta-api/common/env"
	"meta-api/common/types"
)

const (
	CommentAccessTokenCookie = "comment_access_token"
	commentAuthCookieMaxAge  = 7 * 24 * 60 * 60
	commentAuthCookiePath    = "/"
	commentUserTokenUse      = "comment_user"
)

func GenerateCommentUserToken(user *types.PublicUserInfo) (string, error) {
	signingKey, err := RequiredEnvOrFile(env.JWTSigningKey)
	if err != nil {
		return "", err
	}
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
	tokenString, err := token.SignedString([]byte(signingKey))
	if err != nil {
		return "", fmt.Errorf("failed to generate comment user token: %w", err)
	}
	return tokenString, nil
}

func ParseCommentUserToken(tokenString string) (*types.PublicUserClaims, error) {
	signingKey, err := RequiredEnvOrFile(env.JWTSigningKey)
	if err != nil {
		return nil, err
	}
	token, err := jwt.ParseWithClaims(tokenString, &types.PublicUserClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
			}
			return []byte(signingKey), nil
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
	secure := IsProductionEnv()
	c.SetSameSite(commentAuthSameSiteMode())
	c.SetCookie(CommentAccessTokenCookie, token, commentAuthCookieMaxAge, commentAuthCookiePath, "", secure, true)
}

func ClearCommentAuthCookie(c *gin.Context) {
	secure := IsProductionEnv()
	c.SetSameSite(commentAuthSameSiteMode())
	c.SetCookie(CommentAccessTokenCookie, "", -1, commentAuthCookiePath, "", secure, true)
}

func commentAuthSameSiteMode() http.SameSite {
	if IsProductionEnv() {
		return http.SameSiteLaxMode
	}
	return http.SameSiteLaxMode
}
