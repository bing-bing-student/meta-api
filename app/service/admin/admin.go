package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/admin"
	"meta-api/common/cachekey"
	"meta-api/common/env"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/common/utils"
	"meta-api/pkg/sms"
)

const loginChallengeTTL = 3 * time.Minute

const (
	adminAccessTokenTTL  = 10 * time.Minute
	adminRefreshTokenTTL = 7 * 24 * time.Hour

	adminSessionUserIDField      = "user_id"
	adminSessionRefreshHashField = "refresh_hash"
	adminSessionRefreshJTIField  = "refresh_jti"
)

var (
	errAdminSessionNotFound = errors.New("admin session not found")
	errAdminSessionMismatch = errors.New("admin session mismatch")
	errRefreshTokenRotated  = errors.New("refresh token has been rotated")
)

var adminSessionRotateScript = redis.NewScript(`
local current_user = redis.call("HGET", KEYS[1], ARGV[1])
local current_hash = redis.call("HGET", KEYS[1], ARGV[2])
if not current_user or not current_hash then
	return 0
end
if current_user ~= ARGV[3] then
	return -1
end
if current_hash ~= ARGV[4] then
	return -2
end
redis.call("HSET", KEYS[1],
	ARGV[1], ARGV[3],
	ARGV[2], ARGV[5],
	ARGV[6], ARGV[7])
redis.call("PEXPIRE", KEYS[1], ARGV[8])
return 1
`)

var compareAndDeleteScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if not current then
	return 0
end
if current ~= ARGV[1] then
	return -1
end
redis.call("DEL", KEYS[1])
return 1
`)

var consumeDynamicCodeChallengeScript = redis.NewScript(`
local current_user = redis.call("GET", KEYS[1])
if not current_user or current_user ~= ARGV[1] then
	return 0
end
if ARGV[2] ~= "" then
	local current_secret = redis.call("GET", KEYS[2])
	if not current_secret or current_secret ~= ARGV[2] then
		return 0
	end
end
redis.call("DEL", unpack(KEYS))
return 1
`)

// generateLoginChallenge 生成二阶段登录用的一次性随机挑战值。
func generateLoginChallenge() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// createLoginChallenge 保存账号密码校验后的短期登录挑战。
func (a *adminService) createLoginChallenge(ctx context.Context, userID string) (string, error) {
	challenge, err := generateLoginChallenge()
	if err != nil {
		return "", err
	}
	key := cachekey.AdminLoginChallenge(challenge).String()
	if err = a.redis.Set(ctx, key, userID, loginChallengeTTL).Err(); err != nil {
		return "", err
	}
	return challenge, nil
}

// getLoginChallengeUserID 根据登录挑战反查待验证用户。
func (a *adminService) getLoginChallengeUserID(ctx context.Context, challenge string) (string, error) {
	key := cachekey.AdminLoginChallenge(challenge).String()
	userID, err := a.redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errors.New("登录状态已过期，请重新输入账号密码")
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

// clearLoginChallenge 清理未完成的登录挑战。
func (a *adminService) clearLoginChallenge(ctx context.Context, challenge string) {
	if challenge == "" {
		return
	}
	if err := a.redis.Del(ctx, cachekey.AdminLoginChallenge(challenge).String()).Err(); err != nil {
		a.logger.Warn("failed to clear login challenge", zap.Error(err))
	}
}

// GenerateToken 生成AccessToken和RefreshToken，并把当前 refresh token 状态写入 Redis。
func (a *adminService) GenerateToken(ctx context.Context, userClaims *types.UserClaims) (*types.TokenDetails, error) {
	if userClaims == nil || userClaims.UserID == "" {
		return nil, errors.New("invalid user claims")
	}
	return a.issueTokenPair(ctx, userClaims.UserID, "", "")
}

func (a *adminService) RefreshToken(ctx context.Context, refreshToken string) (*types.TokenDetails, error) {
	claims, err := utils.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	doubleToken, err := a.issueTokenPair(ctx, claims.UserID, claims.SessionID, hashRefreshToken(refreshToken))
	if err != nil {
		if errors.Is(err, errAdminSessionMismatch) || errors.Is(err, errRefreshTokenRotated) {
			if delErr := a.redis.Del(ctx, cachekey.AdminSession(claims.SessionID).String()).Err(); delErr != nil {
				a.logger.Warn("failed to revoke invalid admin session", zap.Error(delErr))
			}
		}
		return nil, err
	}

	return doubleToken, nil
}

func (a *adminService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	claims, err := utils.ParseRefreshToken(refreshToken)
	if err != nil {
		return err
	}
	if err = a.redis.Del(ctx, cachekey.AdminSession(claims.SessionID).String()).Err(); err != nil {
		return err
	}
	return nil
}

func (a *adminService) issueTokenPair(ctx context.Context, userID string, sessionID string,
	expectedRefreshHash string) (*types.TokenDetails, error) {
	tokenDetails := &types.TokenDetails{}
	signingKey, err := utils.RequiredEnvOrFile(env.JWTSigningKey)
	if err != nil {
		return nil, err
	}
	mySigningKey := []byte(signingKey)
	now := time.Now()
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 访问令牌15分钟后过期
	tokenDetails.AtExpires = now.Add(adminAccessTokenTTL).Unix()
	tokenDetails.AccessUUID = uuid.New().String()

	// 创建访问令牌的声明
	accessTokenClaims := &types.UserClaims{
		UserID:    userID,
		TokenUse:  utils.AdminAccessTokenUse,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Unix(tokenDetails.AtExpires, 0)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenDetails.AccessUUID,
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
	tokenDetails.RtExpires = now.Add(adminRefreshTokenTTL).Unix()
	tokenDetails.RefreshUUID = uuid.New().String()

	// 创建刷新令牌的声明
	refreshTokenClaims := &types.UserClaims{
		UserID:    userID,
		TokenUse:  utils.AdminRefreshTokenUse,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Unix(tokenDetails.RtExpires, 0)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenDetails.RefreshUUID,
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

	if err = a.storeAdminSession(ctx, sessionID, userID, tokenDetails.RefreshUUID,
		hashRefreshToken(refreshTokenString), expectedRefreshHash); err != nil {
		return nil, err
	}

	return tokenDetails, nil
}

func (a *adminService) storeAdminSession(ctx context.Context, sessionID string, userID string, refreshJTI string,
	refreshHash string, expectedRefreshHash string) error {
	sessionKey := cachekey.AdminSession(sessionID).String()
	sessionFields := map[string]any{
		adminSessionUserIDField:      userID,
		adminSessionRefreshHashField: refreshHash,
		adminSessionRefreshJTIField:  refreshJTI,
	}
	if expectedRefreshHash == "" {
		_, err := a.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, sessionKey, sessionFields)
			pipe.Expire(ctx, sessionKey, adminRefreshTokenTTL)
			return nil
		})
		return err
	}

	result, err := adminSessionRotateScript.Run(ctx, a.redis, []string{sessionKey},
		adminSessionUserIDField,
		adminSessionRefreshHashField,
		userID,
		expectedRefreshHash,
		refreshHash,
		adminSessionRefreshJTIField,
		refreshJTI,
		adminRefreshTokenTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	switch result {
	case 1:
		return nil
	case -1:
		return errAdminSessionMismatch
	case -2:
		return errRefreshTokenRotated
	default:
		return errAdminSessionNotFound
	}
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SendSMSCode 发送短信验证码
func (a *adminService) SendSMSCode(ctx context.Context, request *types.SendSMSCodeRequest) error {
	// 校验手机号
	if userID, err := a.model.PhoneNumberExist(ctx, request.Phone); userID == "" || err != nil {
		a.logger.Error("invalid mobile number", zap.Error(err))
		return fmt.Errorf("invalid mobile number")
	}

	// 发送验证码
	code, err := sms.SendMessage(request.Phone)
	if err != nil {
		a.logger.Error("failed to send sms code", zap.Error(err))
		return err
	}

	// 缓存验证码（按手机号隔离，避免并发请求互相覆盖）
	if err = a.redis.Set(ctx, cachekey.SMSCode(request.Phone).String(), code, time.Minute).Err(); err != nil {
		a.logger.Error("failed to cache sms code", zap.Error(err))
		return err
	}
	return nil
}

// SMSCodeLogin 短信验证码登录
func (a *adminService) SMSCodeLogin(ctx context.Context,
	request *types.SMSCodeLoginRequest) (*types.SMSCodeLoginResponse, error) {

	// 原子校验并消费短信验证码，避免两个并发登录请求同时通过 GET 与 DEL
	// 之间的窗口。错误验证码不会删除仍然有效的正确验证码。
	response := &types.SMSCodeLoginResponse{}
	smsKey := cachekey.SMSCode(request.Phone).String()
	consumeResult, err := compareAndDeleteScript.Run(ctx, a.redis, []string{smsKey}, request.Code).Int64()
	if err != nil {
		a.logger.Error("failed to consume sms verification code", zap.Error(err))
		return response, errors.New("登录服务暂不可用")
	}
	switch consumeResult {
	case 0:
		return response, errors.New("sms verification code does not exist")
	case -1:
		return response, errors.New("sms verification code error")
	}

	// 校验手机号
	userID, err := a.model.PhoneNumberExist(ctx, request.Phone)
	if userID == "" || err != nil {
		a.logger.Error("invalid mobile number", zap.Error(err))
		return response, fmt.Errorf("invalid mobile number")
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(ctx, claims)
	if err != nil {
		a.logger.Error("failed to generate new tokens", zap.Error(err))
		return response, fmt.Errorf("failed to generate new tokens")
	}
	response.UserID = userID
	response.AccessToken = doubleToken.AccessToken
	response.RefreshToken = doubleToken.RefreshToken

	return response, nil
}

// AccountLogin 账号密码登录
func (a *adminService) AccountLogin(ctx context.Context,
	request *types.AccountLoginRequest) (*types.AccountLoginResponse, error) {

	// 查询用户名和密码是否正确
	response := &types.AccountLoginResponse{}
	if err := a.checkAccountLoginLimit(ctx, request); err != nil {
		a.logger.Warn("account login rate limited", zap.Error(err))
		return nil, err
	}
	adminInfo, err := a.model.CheckAccount(ctx, request.Username, request.Password)
	if err != nil {
		a.logger.Error("incorrect account or password", zap.Error(err))
		if limitErr := a.recordAccountLoginFailure(ctx, request.Username); limitErr != nil {
			return nil, limitErr
		}
		return nil, err
	}
	a.clearAccountLoginState(ctx, request.Username)

	userID := strconv.FormatUint(adminInfo.ID, 10)
	loginChallenge, err := a.createLoginChallenge(ctx, userID)
	if err != nil {
		a.logger.Error("failed to create login challenge", zap.Error(err))
		return nil, errors.New("登录状态初始化失败")
	}
	response.LoginChallenge = loginChallenge

	// 如果用户未绑定TOTP，则生成TOTP密钥和二维码URL
	if adminInfo.BindStatus == 0 && adminInfo.SecretKey == "" {
		adminInfoConfig := a.config.AdminInfoSnapshot()
		issuer := adminInfoConfig.Issuer
		accountName := adminInfoConfig.AccountName
		secret, qrCodeURL, err := utils.GenerateTOTP(issuer, accountName)
		if err != nil {
			a.logger.Error("failed to generate TOTP", zap.Error(err))
			a.clearLoginChallenge(ctx, loginChallenge)
			return response, errors.New("生成 TOTP 密钥和二维码URL失败")
		}

		key := cachekey.AdminPendingTOTPSecret(loginChallenge).String()
		if err = a.redis.Set(ctx, key, secret, loginChallengeTTL).Err(); err != nil {
			a.logger.Error("failed to store TOTP secret key in Redis", zap.Error(err))
			a.clearLoginChallenge(ctx, loginChallenge)
			return nil, errors.New("生成 TOTP 密钥和二维码 URL 失败")
		}
		response.QRCodeURL = qrCodeURL
		return response, nil
	}

	return response, nil
}

// BindDynamicCode 绑定动态码
func (a *adminService) BindDynamicCode(ctx context.Context,
	request *types.BindDynamicCodeRequest) (*types.BindDynamicCodeResponse, error) {

	// 检查密钥是否存在并验证
	response := new(types.BindDynamicCodeResponse)
	if err := a.checkBindDynamicCodeLimit(ctx, request); err != nil {
		a.logger.Warn("bind dynamic code rate limited", zap.Error(err))
		return response, err
	}
	userID, err := a.getLoginChallengeUserID(ctx, request.LoginChallenge)
	if err != nil {
		a.logger.Error("invalid login challenge", zap.Error(err))
		return response, err
	}

	key := cachekey.AdminPendingTOTPSecret(request.LoginChallenge).String()
	secretKey, err := a.redis.Get(ctx, key).Result()
	if err != nil {
		a.logger.Error("failed to get secret key from Redis", zap.Error(err))
		return response, errors.New("failed to get secret key from Redis")
	}
	if !utils.VerifyTOTP(request.Code, secretKey) {
		a.logger.Error("failed to verify TOTP", zap.Error(err))
		if limitErr := a.recordBindDynamicCodeFailure(ctx, request.LoginChallenge); limitErr != nil {
			return response, limitErr
		}
		return response, errors.New("无效的动态验证码")
	}

	id, err := idutil.ParseID("userID", userID)
	if err != nil {
		a.logger.Error("invalid userID", zap.Error(err))
		return response, errors.New("invalid userID")
	}
	if err = a.consumeDynamicCodeChallenge(ctx, request.LoginChallenge, userID, secretKey); err != nil {
		a.logger.Warn("failed to consume TOTP bind challenge", zap.Error(err))
		return response, err
	}
	if err = a.model.AddAdminSecretKey(ctx, id, secretKey); err != nil {
		a.logger.Error("failed to add secret key to database", zap.Error(err))
		return response, errors.New("failed to add secret key to database")
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(ctx, claims)
	if err != nil {
		a.logger.Error("failed to generate new tokens", zap.Error(err))
		return response, fmt.Errorf("failed to generate new tokens")
	}
	response.UserID = userID
	response.AccessToken = doubleToken.AccessToken
	response.RefreshToken = doubleToken.RefreshToken

	return response, nil
}

// VerifyDynamicCode 验证动态码
func (a *adminService) VerifyDynamicCode(ctx context.Context,
	request *types.VerifyDynamicCodeRequest) (*types.VerifyDynamicCodeResponse, error) {

	// 从 mysql 当中获取 secretKey 并进行验证
	response := &types.VerifyDynamicCodeResponse{}
	if err := a.checkVerifyDynamicCodeLimit(ctx, request); err != nil {
		a.logger.Warn("verify dynamic code rate limited", zap.Error(err))
		return response, err
	}
	userID, err := a.getLoginChallengeUserID(ctx, request.LoginChallenge)
	if err != nil {
		a.logger.Error("invalid login challenge", zap.Error(err))
		return response, err
	}

	id, err := idutil.ParseID("userID", userID)
	if err != nil {
		a.logger.Error("invalid userID", zap.Error(err))
		return response, errors.New("invalid userID")
	}
	secretKey, err := a.model.GetAdminSecretKey(ctx, id)
	if err != nil {
		a.logger.Error("failed to get secret key from database", zap.Error(err))
		return response, errors.New("failed to get secret key from database")
	}
	if !utils.VerifyTOTP(request.Code, secretKey) {
		a.logger.Error("failed to verify TOTP", zap.Error(err))
		if limitErr := a.recordVerifyDynamicCodeFailure(ctx, request.LoginChallenge); limitErr != nil {
			return response, limitErr
		}
		return response, errors.New("无效的动态验证码")
	}
	if err = a.consumeDynamicCodeChallenge(ctx, request.LoginChallenge, userID, ""); err != nil {
		a.logger.Warn("failed to consume TOTP login challenge", zap.Error(err))
		return response, err
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(ctx, claims)
	if err != nil {
		a.logger.Error("failed to generate new tokens", zap.Error(err))
		return response, fmt.Errorf("failed to generate new tokens")
	}
	response.UserID = userID
	response.AccessToken = doubleToken.AccessToken
	response.RefreshToken = doubleToken.RefreshToken

	return response, nil
}

// AdminUpdateAboutMe 修改关于我
func (a *adminService) AdminUpdateAboutMe(ctx context.Context, request *types.UpdateAboutMeRequest) error {
	// 获取管理员信息
	id, err := idutil.ParseID("userID", request.UserID)
	if err != nil {
		a.logger.Error("invalid userID", zap.Error(err))
		return fmt.Errorf("invalid userID: %w", err)
	}
	adminInfo, err := a.model.GetAdminInfoByID(ctx, id)
	if err != nil {
		a.logger.Error("failed to get admin info", zap.Error(err))
		return fmt.Errorf("failed to get admin info")
	}

	aboutMeInfo := admin.AboutMeInfo{}
	if err = utils.JsonStringToStruct(adminInfo.AboutMeInfo, &aboutMeInfo); err != nil {
		a.logger.Error("failed to unmarshal aboutMeInfo", zap.Error(err))
		return err
	}
	if request.Name != "" {
		aboutMeInfo.Name = request.Name
	}
	if request.Job != "" {
		aboutMeInfo.Job = request.Job
	}
	if request.Address != "" {
		aboutMeInfo.Address = request.Address
	}
	if request.WorkLife != "" {
		aboutMeInfo.WorkLife = request.WorkLife
	}

	var webSiteInfo admin.WebSiteInfo
	if err = utils.JsonStringToStruct(adminInfo.WebSiteInfo, &webSiteInfo); err != nil {
		a.logger.Error("failed to unmarshal webSiteInfo", zap.Error(err))
		return err
	}
	if request.Statement != "" {
		webSiteInfo.Statement = request.Statement
	}
	if request.DomainInfo != "" {
		webSiteInfo.DomainInfo = request.DomainInfo
	}
	if request.BlogContent != "" {
		webSiteInfo.BlogContent = request.BlogContent
	}
	if request.WebsiteLocation != "" {
		webSiteInfo.WebsiteLocation = request.WebsiteLocation
	}

	var contactMeInfo admin.ContactMeInfo
	if err = utils.JsonStringToStruct(adminInfo.ContactMeInfo, &contactMeInfo); err != nil {
		a.logger.Error("failed to unmarshal contactMeInfo", zap.Error(err))
		return err
	}
	if len(request.Email) > 0 {
		contactMeInfo.Email = request.Email
	}

	aboutMeInfoStr, err := utils.StructToJsonString(aboutMeInfo)
	if err != nil {
		a.logger.Error("jsonToString error for aboutMeInfo", zap.Error(err))
		return err
	}
	webSiteInfoStr, err := utils.StructToJsonString(webSiteInfo)
	if err != nil {
		a.logger.Error("jsonToString error for webSiteInfo", zap.Error(err))
		return err
	}
	contactMeInfoStr, err := utils.StructToJsonString(contactMeInfo)
	if err != nil {
		a.logger.Error("jsonToString error for contactMeInfo", zap.Error(err))
		return err
	}

	// 更新数据库
	updatedAdminInfo := admin.Admin{
		AboutMeInfo:   aboutMeInfoStr,
		WebSiteInfo:   webSiteInfoStr,
		ContactMeInfo: contactMeInfoStr,
	}
	if err = a.model.UpdateAdminInfoByID(ctx, id, &updatedAdminInfo); err != nil {
		a.logger.Error("failed to update admin info", zap.Error(err))
	}

	// 删除缓存
	if err = a.redis.Del(ctx, cachekey.AboutMeHash().String()).Err(); err != nil {
		a.logger.Error("failed to clear aboutMeInfo cache", zap.Error(err))
		return err
	}
	return nil
}
