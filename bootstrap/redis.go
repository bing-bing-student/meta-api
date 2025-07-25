package bootstrap

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"meta-api/common/utils"
	"meta-api/config"
)

// ConnectRedisClient 初始化Redis客户端
func ConnectRedisClient(ctx context.Context, cfg *RedisConfig) (*redis.Client, error) {
	client := redis.NewFailoverClient(&redis.FailoverOptions{
		Password:      os.Getenv("REDIS_PASSWORD"),
		DB:            cfg.RedisConfig.DB,
		MasterName:    os.Getenv("REDIS_MASTER_NAME"),
		SentinelAddrs: strings.Split(os.Getenv("REDIS_ADDRESS"), ","),
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

type RedisConfig struct {
	RedisConfig *config.RedisConfig
	RetryConfig *config.RetryConfig
}

// Redis 初始化Redis
func initRedis(cfg *RedisConfig) *redis.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var client *redis.Client
	var err error
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		client, err = ConnectRedisClient(ctx, cfg)
		return nil
	}); err != nil {
		panic("Redis connection failed: " + err.Error())
	}

	return client
}
