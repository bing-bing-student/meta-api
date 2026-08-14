package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"meta-api/common/env"
	"meta-api/common/utils"
)

const (
	localDotEnvFile = ".env"
)

// RuntimeEnv 是启动阶段读取并标准化后的运行时环境变量。
type RuntimeEnv struct {
	HTTPHost string
	HTTPPort string
}

// LoadStartupRuntimeEnv 加载启动期运行环境。
//
// 本地开发允许从 .env 补齐环境变量；生产环境必须由容器/进程管理器显式注入，
// 避免线上进程意外读取工作目录中的 .env。
func LoadStartupRuntimeEnv() (*RuntimeEnv, error) {
	if err := LoadLocalDotEnv(); err != nil {
		return nil, err
	}
	return LoadRuntimeEnv()
}

// LoadLocalDotEnv 仅在非生产环境加载本地 .env。
func LoadLocalDotEnv() error {
	if utils.IsProductionEnv() {
		return nil
	}
	if _, err := os.Stat(localDotEnvFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat local %s: %w", localDotEnvFile, err)
	}
	if err := godotenv.Load(localDotEnvFile); err != nil {
		return fmt.Errorf("load local %s: %w", localDotEnvFile, err)
	}
	return nil
}

// LoadRuntimeEnv 集中校验启动所需环境变量，并返回标准化后的运行时配置。
func LoadRuntimeEnv() (*RuntimeEnv, error) {
	required := []string{
		env.HTTPHost,
		env.HTTPPort,
		env.SonyflakeMachineID,
		env.MySQLUsername,
		env.MySQLHost,
		env.MySQLPort,
		env.MySQLDBName,
		env.RedisMasterName,
		env.RedisAddress,
	}
	values, err := requiredEnvValues(required...)
	if err != nil {
		return nil, err
	}

	httpPort, err := normalizedHTTPPort(values[env.HTTPPort])
	if err != nil {
		return nil, err
	}
	if _, err = utils.RequiredEnvOrFile(env.JWTSigningKey); err != nil {
		return nil, err
	}
	if _, err = utils.RequiredEnvOrFile(env.MySQLWorkPassword); err != nil {
		return nil, err
	}
	if utils.IsProductionEnv() {
		if _, err = utils.RequiredEnvOrFile(env.RedisPassword); err != nil {
			return nil, err
		}
	}

	return &RuntimeEnv{
		HTTPHost: normalizedHTTPHost(values[env.HTTPHost]),
		HTTPPort: httpPort,
	}, nil
}

func normalizedHTTPHost(host string) string {
	return strings.TrimSpace(host)
}

func normalizedHTTPPort(port string) (string, error) {
	port = strings.TrimSpace(port)
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return "", fmt.Errorf("invalid %s: %q", env.HTTPPort, port)
	}
	return port, nil
}

func requiredEnvValues(names ...string) (map[string]string, error) {
	values := make(map[string]string, len(names))
	var missing []string
	for _, name := range names {
		value := os.Getenv(name)
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
			continue
		}
		values[name] = value
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return values, nil
}

func splitEnvList(name string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("missing required environment variable: %s", name)
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("missing required environment variable: %s", name)
	}
	return values, nil
}
