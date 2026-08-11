package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"meta-api/common/utils"
	"meta-api/config"
)

const (
	envRedisAddress    = "REDIS_ADDRESS"
	envRedisMasterName = "REDIS_MASTER_NAME"
	envRedisPassword   = "REDIS_PASSWORD"
)

// ConnectRedisClient 初始化Redis客户端
func ConnectRedisClient(ctx context.Context, cfg *RedisConfig) (*redis.Client, error) {
	if err := validateRedisConfig(cfg); err != nil {
		return nil, err
	}
	env, err := redisEnvFromEnv()
	if err != nil {
		return nil, err
	}
	return connectRedisClient(ctx, cfg, env)
}

func connectRedisClient(ctx context.Context, cfg *RedisConfig, env *redisEnv) (*redis.Client, error) {
	client := redis.NewFailoverClient(&redis.FailoverOptions{
		Password:      env.password,
		DB:            cfg.RedisConfig.DB,
		MasterName:    env.masterName,
		SentinelAddrs: env.sentinelAddrs,
	})

	// Ping 失败时必须显式关闭 client：
	// redis.NewFailoverClient 一调用就会预先建立连接池 + 启动 sentinel 订阅协程，
	// 即使 Ping 失败这些资源也仍存活；上层 utils.WithBackoff 重试时会反复创建，
	// 不关闭就会累积泄漏 fd 与后台协程。
	if err := client.Ping(ctx).Err(); err != nil {
		if cErr := client.Close(); cErr != nil {
			// 用 errors.Join 把 Close 错误链回 err，让调用方决定是否记录；
			// 这里没有 logger 上下文，最适合的处理是把两条错误都暴露上去。
			return nil, errors.Join(err, cErr)
		}
		return nil, err
	}

	return client, nil
}

type RedisConfig struct {
	RedisConfig *config.RedisConfig
	RetryConfig *config.RetryConfig
}

type redisEnv struct {
	password      string
	masterName    string
	sentinelAddrs []string
}

// Redis 初始化Redis
func initRedis(cfg *RedisConfig) (*redis.Client, error) {
	if err := validateRedisConfig(cfg); err != nil {
		return nil, err
	}

	env, err := redisEnvFromEnv()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var client *redis.Client
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		client, err = connectRedisClient(ctx, cfg, env)
		return err
	}); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return client, nil
}

func validateRedisConfig(cfg *RedisConfig) error {
	if cfg == nil {
		return fmt.Errorf("redis config is nil")
	}
	if cfg.RedisConfig == nil {
		return fmt.Errorf("redis connection config is nil")
	}
	if cfg.RetryConfig == nil {
		return fmt.Errorf("redis retry config is nil")
	}
	return nil
}

func redisEnvFromEnv() (*redisEnv, error) {
	values, err := requiredEnvValues(envRedisMasterName)
	if err != nil {
		return nil, err
	}

	sentinelAddrs, err := splitEnvList(envRedisAddress)
	if err != nil {
		return nil, err
	}

	password := strings.TrimSpace(os.Getenv(envRedisPassword))
	if password == "" && utils.IsProductionEnv() {
		return nil, fmt.Errorf("missing required environment variable: %s", envRedisPassword)
	}

	return &redisEnv{
		password:      password,
		masterName:    values[envRedisMasterName],
		sentinelAddrs: sentinelAddrs,
	}, nil
}
