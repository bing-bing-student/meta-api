package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/admin"
	"meta-api/common/cachekey"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/common/utils"
	"meta-api/pkg/sms"
)

const loginChallengeTTL = 3 * time.Minute

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

// GenerateToken 生成AccessToken和RefreshToken
func (a *adminService) GenerateToken(userClaims *types.UserClaims) (*types.TokenDetails, error) {
	tokenDetails := &types.TokenDetails{}
	mySigningKey := []byte(os.Getenv("JWT_SIGNING_KEY"))

	// 访问令牌15分钟后过期
	tokenDetails.AtExpires = time.Now().Add(time.Minute * 15).Unix()
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

	// 校验短信验证码（按手机号取对应缓存）
	response := &types.SMSCodeLoginResponse{}
	smsKey := cachekey.SMSCode(request.Phone).String()
	smsCode, err := a.redis.Get(ctx, smsKey).Result()
	if err != nil {
		a.logger.Error("sms verification code does not exist", zap.Error(err))
		return response, errors.New("sms verification code does not exist")
	}
	if request.Code != smsCode {
		a.logger.Error("sms verification code error", zap.Error(err))
		return response, errors.New("sms verification code error")
	}

	// 校验手机号
	userID, err := a.model.PhoneNumberExist(ctx, request.Phone)
	if userID == "" || err != nil {
		a.logger.Error("invalid mobile number", zap.Error(err))
		return response, fmt.Errorf("invalid mobile number")
	}

	// 验证码一次性使用，登录成功后立即删除，防止重放
	if err = a.redis.Del(ctx, smsKey).Err(); err != nil {
		// 删除失败只记录日志，不影响登录流程
		a.logger.Warn("failed to delete sms code after login", zap.Error(err))
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(claims)
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
	if err = a.model.AddAdminSecretKey(ctx, id, secretKey); err != nil {
		a.logger.Error("failed to add secret key to database", zap.Error(err))
		return response, errors.New("failed to add secret key to database")
	}
	if err = a.clearDynamicCodeState(ctx, request.LoginChallenge, key); err != nil {
		a.logger.Error("failed to clear TOTP login challenge", zap.Error(err))
		return response, errors.New("failed to clear TOTP login challenge")
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(claims)
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
	if err = a.clearDynamicCodeState(ctx, request.LoginChallenge); err != nil {
		a.logger.Error("failed to clear TOTP login challenge", zap.Error(err))
		return response, errors.New("failed to clear TOTP login challenge")
	}

	// 生成双 Token
	claims := new(types.UserClaims)
	claims.UserID = userID
	doubleToken, err := a.GenerateToken(claims)
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
