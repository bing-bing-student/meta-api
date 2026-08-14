package utils

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"meta-api/common/env"
	"meta-api/common/types"
)

const (
	AdminAccessTokenUse  = "admin_access"
	AdminRefreshTokenUse = "admin_refresh"
)

// ParseToken 解析Token
func ParseToken(tokenString string) (*types.UserClaims, error) {
	signingKey, err := RequiredEnvOrFile(env.JWTSigningKey)
	if err != nil {
		return nil, err
	}
	mySigningKey := []byte(signingKey)

	token, err := jwt.ParseWithClaims(tokenString, &types.UserClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
			}
			return mySigningKey, nil
		},
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("TokenExpired")
		}
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if token == nil {
		return nil, errors.New("token is null")
	}
	if !token.Valid {
		return nil, errors.New("token is invalid")
	}

	claims, ok := token.Claims.(*types.UserClaims)
	if !ok {
		return nil, fmt.Errorf("token claims are of incorrect type: %T", token.Claims)
	}

	return claims, nil
}

func ParseAccessToken(tokenString string) (*types.UserClaims, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenUse != AdminAccessTokenUse {
		return nil, errors.New("invalid token use")
	}
	return claims, nil
}

func ParseRefreshToken(tokenString string) (*types.UserClaims, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenUse != AdminRefreshTokenUse || claims.SessionID == "" || claims.ID == "" {
		return nil, errors.New("invalid token use")
	}
	return claims, nil
}
