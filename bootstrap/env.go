package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"meta-api/common/utils"
)

const (
	envHTTPHost      = "HTTP_HOST"
	envHTTPPort      = "HTTP_PORT"
	envJWTSigningKey = "JWT_SIGNING_KEY"

	defaultHTTPHost = "0.0.0.0"
	defaultHTTPPort = "8080"
)

// RuntimeEnv 是启动阶段读取并标准化后的运行时环境变量。
//
// 仅放启动期需要稳定下来的值；业务可选能力（CDN/COS/Sitemap/OAuth secret 等）
// 仍由对应客户端或 service 在调用处读取，保持"缺失则禁用"的现有语义。
type RuntimeEnv struct {
	HTTPHost string
	HTTPPort string
}

// LoadRuntimeEnv 集中校验启动所需环境变量，并返回标准化后的运行时配置。
func LoadRuntimeEnv() (*RuntimeEnv, error) {
	httpPort, err := normalizedHTTPPort()
	if err != nil {
		return nil, err
	}

	required := []string{
		envMySQLUsername,
		envMySQLHost,
		envMySQLPort,
		envMySQLDBName,
		envRedisMasterName,
		envRedisAddress,
	}
	if _, err = requiredEnvValues(required...); err != nil {
		return nil, err
	}
	if _, err = utils.RequiredEnvOrFile(envJWTSigningKey); err != nil {
		return nil, err
	}
	if _, err = utils.RequiredEnvOrFile(envMySQLPassword); err != nil {
		return nil, err
	}
	if utils.IsProductionEnv() {
		if _, err = utils.RequiredEnvOrFile(envRedisPassword); err != nil {
			return nil, err
		}
	}

	return &RuntimeEnv{
		HTTPHost: normalizedHTTPHost(),
		HTTPPort: httpPort,
	}, nil
}

func defaultRuntimeEnv() RuntimeEnv {
	return RuntimeEnv{
		HTTPHost: defaultHTTPHost,
		HTTPPort: defaultHTTPPort,
	}
}

func normalizedHTTPHost() string {
	host := strings.TrimSpace(os.Getenv(envHTTPHost))
	if host == "" {
		return defaultHTTPHost
	}
	return host
}

func normalizedHTTPPort() (string, error) {
	port := strings.TrimSpace(os.Getenv(envHTTPPort))
	if port == "" {
		port = defaultHTTPPort
	}
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return "", fmt.Errorf("invalid %s: %q", envHTTPPort, port)
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
