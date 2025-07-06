package utils

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"

	"meta-api/internal/common/types"
)

// ParseToken 解析Token
func ParseToken(tokenString string) (*types.UserClaims, error) {
	mySigningKey := []byte(os.Getenv("JWT_SIGNING_KEY"))

	token, err := jwt.ParseWithClaims(tokenString, &types.UserClaims{},
		func(token *jwt.Token) (interface{}, error) {
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

	claims, ok := token.Claims.(*types.UserClaims)
	if !ok {
		return nil, fmt.Errorf("token claims are of incorrect type: %T", token.Claims)
	}

	return claims, nil
}
