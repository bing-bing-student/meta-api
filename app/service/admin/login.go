package admin

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"meta-api/common/types"
)

// GenerateTokenService 生成AccessToken和RefreshToken
func (a *adminService) GenerateTokenService(userClaims *types.UserClaims) (*types.TokenDetails, error) {
	tokenDetails := &types.TokenDetails{}
	mySigningKey := []byte(os.Getenv("JWT_SIGNING_KEY"))

	// 访问令牌1小时后过期
	tokenDetails.AtExpires = time.Now().Add(time.Hour * 1).Unix()
	tokenDetails.AccessUUID = uuid.New().String()

	// 创建访问令牌的声明
	accessTokenClaims := &types.UserClaims{
		UserID: userClaims.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Unix(tokenDetails.AtExpires, 0)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// 创建访问令牌
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenClaims)
	accessTokenString, err := accessToken.SignedString(mySigningKey)
	if err != nil {
		a.logger.Error("failed to generate access token", zap.Error(err))
		return nil, err
	}
	tokenDetails.AccessToken = accessTokenString

	// 刷新令牌7天后过期
	tokenDetails.RtExpires = time.Now().Add(time.Hour * 24 * 7).Unix()
	tokenDetails.RefreshUUID = uuid.New().String()

	// 创建刷新令牌的声明
	refreshTokenClaims := &types.UserClaims{
		UserID: userClaims.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Unix(tokenDetails.RtExpires, 0)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// 创建刷新令牌
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshTokenClaims)
	refreshTokenString, err := refreshToken.SignedString(mySigningKey)
	if err != nil {
		a.logger.Error("failed to generate refresh token", zap.Error(err))
		return nil, err
	}
	tokenDetails.RefreshToken = refreshTokenString

	return tokenDetails, nil
}
