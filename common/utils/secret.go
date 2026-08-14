package utils

import (
	"fmt"
	"os"
	"strings"

	"meta-api/common/env"
)

// EnvOrFile 读取敏感配置，优先使用 <name>_FILE 指向的文件，其次使用 <name> 环境变量。
//
// 这让线上容器可以通过 Docker secrets/K8s secrets 只读文件注入敏感值，
// 避免 secret 长期出现在容器环境变量中；本地开发仍可使用普通环境变量兜底。
func EnvOrFile(name string) (string, error) {
	return EnvOrFileWithFile(name, env.File(name))
}

// EnvOrFileWithFile 读取敏感配置，优先使用 fileName 指向的文件，其次使用 name 环境变量。
func EnvOrFileWithFile(name string, fileName string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(fileName)); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read secret file from %s: %w", fileName, err)
		}
		value := strings.TrimSpace(string(content))
		if value == "" {
			return "", fmt.Errorf("secret file from %s is empty", fileName)
		}
		return value, nil
	}
	return strings.TrimSpace(os.Getenv(name)), nil
}

// RequiredEnvOrFile 读取必需敏感配置，未配置时返回错误。
func RequiredEnvOrFile(name string) (string, error) {
	value, err := EnvOrFile(name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("missing required secret: %s or %s", name, env.File(name))
	}
	return value, nil
}
