package utils

import (
	"encoding/base64"
	"fmt"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
)

// GenerateTOTP 生成TOTP密钥和二维码URL
func GenerateTOTP(issuer, accountName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		SecretSize:  32,
		Algorithm:   otp.AlgorithmSHA256,
	})
	if err != nil {
		return "", "", err
	}

	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return "", "", err
	}

	// 将二维码图像转换为 base64 编码的字符串
	base64QRCode := base64.StdEncoding.EncodeToString(png)
	qrCodeURL := fmt.Sprintf("data:image/png;base64,%s", base64QRCode)
	return key.Secret(), qrCodeURL, nil
}

// VerifyTOTP 验证TOTP
func VerifyTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}
