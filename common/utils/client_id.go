package utils

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"regexp"
)

// CheckClientID 校验clientID的utils函数
func CheckClientID(xClientID string) (string, bool) {
	keyPath := "/home/work/meta-api/private_key.pem"
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return "", false
	}
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return "", false
	}
	if xClientID == "" {
		return "", false
	}
	decodedClientID, err := base64.StdEncoding.DecodeString(xClientID)
	if err != nil {
		return "", false
	}
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return "", false
	}
	privateKeyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", false
	}
	rsaPrivate, ok := privateKeyInterface.(*rsa.PrivateKey)
	if !ok {
		return "", false
	}
	plaintext, err := rsa.DecryptPKCS1v15(nil, rsaPrivate, decodedClientID)
	if err != nil {
		return "", false
	}
	if !IsValidClientID(string(plaintext)) {
		return "", false
	}
	return string(plaintext), len(plaintext) == 32
}

// IsValidClientID 验证客户端ID格式
func IsValidClientID(id string) bool {
	if len(id) != 32 {
		return false
	}
	match, _ := regexp.MatchString(`^[a-f0-9]{32}$`, id)
	return match
}
