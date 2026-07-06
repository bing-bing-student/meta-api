package bootstrap

import (
	"context"
	"errors"
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

// Redis 初始化Redis
func initRedis(cfg *RedisConfig) *redis.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var client *redis.Client
	var err error
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		client, err = ConnectRedisClient(ctx, cfg)
		return err
	}); err != nil {
		panic("Redis connection failed: " + err.Error())
	}

	return client
}
