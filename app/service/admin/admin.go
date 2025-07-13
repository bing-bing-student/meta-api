package admin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"meta-api/app/model/admin"
	"meta-api/common/types"
	"meta-api/common/utils"
	"meta-api/pkg/sms"
)

// GenerateToken 生成AccessToken和RefreshToken
func (a *adminService) GenerateToken(userClaims *types.UserClaims) (*types.TokenDetails, error) {
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

	// 缓存验证码
	if err = a.redis.Set(ctx, "code", code, time.Minute).Err(); err != nil {
		a.logger.Error("failed to cache sms code", zap.Error(err))
		return err
	}
	return nil
}

// SMSCodeLogin 短信验证码登录
func (a *adminService) SMSCodeLogin(ctx context.Context,
	request *types.SMSCodeLoginRequest) (*types.SMSCodeLoginResponse, error) {

	// 校验短信验证码
	response := &types.SMSCodeLoginResponse{}
	smsCode, err := a.redis.Get(ctx, "code").Result()
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

	// 生成双Token
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
	adminInfo, err := a.model.CheckAccount(ctx, request.Username, request.Password)
	if err != nil {
		a.logger.Error("incorrect account or password", zap.Error(err))
		return nil, err
	}
	response.UserID = strconv.Itoa(int(adminInfo.ID))

	// 如果用户未绑定TOTP，则生成TOTP密钥和二维码URL
	if adminInfo.BindStatus == 0 && adminInfo.SecretKey == "" {
		issuer := a.config.AdminInfoConfig.Issuer
		accountName := a.config.AdminInfoConfig.AccountName
		secret, qrCodeURL, err := utils.GenerateTOTP(issuer, accountName)
		if err != nil {
			a.logger.Error("failed to generate TOTP", zap.Error(err))
			return response, errors.New("生成TOTP密钥和二维码URL失败")
		}

		key := fmt.Sprintf("admin:%d:secret", adminInfo.ID)
		if err = a.redis.Set(ctx, key, secret, 1*time.Minute).Err(); err != nil {
			a.logger.Error("failed to store TOTP secret key in Redis", zap.Error(err))
			return nil, errors.New("生成TOTP密钥和二维码URL失败")
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
	userID := request.UserID
	key := fmt.Sprintf("admin:%s:secret", userID)
	secretKey, err := a.redis.Get(ctx, key).Result()
	if err != nil {
		a.logger.Error("failed to get secret key from Redis", zap.Error(err))
		return response, errors.New("failed to get secret key from Redis")
	}
	if !utils.VerifyTOTP(request.Code, secretKey) {
		a.logger.Error("failed to verify TOTP", zap.Error(err))
		return response, errors.New("无效的动态验证码")
	}

	// 验证成功，删除Redis中的密钥，并将密码存储到数据库
	if err = a.redis.Del(ctx, key).Err(); err != nil {
		a.logger.Error("failed to delete secret key from Redis", zap.Error(err))
		return response, errors.New("failed to delete secret key from Redis")
	}
	id, err := strconv.Atoi(userID)
	if err != nil {
		a.logger.Error("failed to convert userID to int", zap.Error(err))
		return response, errors.New("failed to convert userID to int")
	}
	if err = a.model.AddAdminSecretKey(ctx, uint64(id), secretKey); err != nil {
		a.logger.Error("failed to add secret key to database", zap.Error(err))
		return response, errors.New("failed to add secret key to database")
	}

	// 生成双Token
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

	// 从mysql当中获取secretKey并进行验证
	response := &types.VerifyDynamicCodeResponse{}
	userID := request.UserID
	id, err := strconv.Atoi(userID)
	if err != nil {
		a.logger.Error("failed to convert userID to int", zap.Error(err))
		return response, errors.New("failed to convert userID to int")
	}
	secretKey, err := a.model.GetAdminSecretKey(ctx, uint64(id))
	if err != nil {
		a.logger.Error("failed to get secret key from database", zap.Error(err))
		return response, errors.New("failed to get secret key from database")
	}
	if !utils.VerifyTOTP(request.Code, secretKey) {
		a.logger.Error("failed to verify TOTP", zap.Error(err))
		return response, errors.New("无效的动态验证码")
	}

	// 生成双Token
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
	id, err := strconv.Atoi(request.UserID)
	if err != nil {
		a.logger.Error("failed to get admin info", zap.Error(err))
		return fmt.Errorf("failed to get admin info")
	}
	adminInfo, err := a.model.GetAdminInfoByID(ctx, uint64(id))
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
	updatedAdminModel := admin.Admin{
		AboutMeInfo:   aboutMeInfoStr,
		WebSiteInfo:   webSiteInfoStr,
		ContactMeInfo: contactMeInfoStr,
	}
	if err = a.model.UpdateAdminInfoByID(ctx, uint64(id), &updatedAdminModel); err != nil {
		a.logger.Error("failed to update admin info", zap.Error(err))
	}

	// 删除缓存
	if err = a.redis.Del(ctx, "aboutMeInfo:Hash").Err(); err != nil {
		a.logger.Error("failed to clear aboutMeInfo cache", zap.Error(err))
		return err
	}
	return nil
}
