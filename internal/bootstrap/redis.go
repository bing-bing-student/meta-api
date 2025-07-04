package bootstrap

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"meta-api/config"
	"meta-api/internal/common/utils"
)

// ConnectRedisClient 初始化Redis客户端
func ConnectRedisClient(ctx context.Context, cfg *RedisConfig) (*redis.Client, error) {
	client := redis.NewFailoverClient(&redis.FailoverOptions{
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
		return err
	}); err != nil {
		panic("Redis connection failed: " + err.Error())
	}

	// 缓存预热的代码放到main函数当中
	//if err = client.Del(ctx, "article:time:ZSet").Err(); err != nil {
	//	logger.Error("failed to delete article:time:ZSet", zap.Error(err))
	//	return nil
	//}
	//if err = client.Del(ctx, "article:view:ZSet").Err(); err != nil {
	//	logger.Error("failed to delete article:view:ZSet", zap.Error(err))
	//	return nil
	//}
	//
	//timeAndViewData := make([]article.TimeAndViewZSet, 0)
	//if err = global.MySqlDB.Model(&article.Article{}).Select("id", "view_num", "create_time").Find(&timeAndViewData).Error; err != nil {
	//	logger.Error("failed to get timeAndViewData", zap.Error(err))
	//	return nil
	//}
	//
	//for _, data := range timeAndViewData {
	//	if err = client.ZAdd(ctx, "article:time:ZSet", redis.Z{
	//		Score:  float64(data.CreateTime.UnixNano() / int64(time.Millisecond)),
	//		Member: data.ID,
	//	}).Err(); err != nil {
	//		logger.Error("failed to add article:time:ZSet", zap.Error(err))
	//		return nil
	//	}
	//	if err = client.ZAdd(ctx, "article:view:ZSet", redis.Z{
	//		Score:  float64(data.ViewNum),
	//		Member: data.ID,
	//	}).Err(); err != nil {
	//		logger.Error("failed to add article:view:ZSet", zap.Error(err))
	//		return nil
	//	}
	//}
	return client
}
