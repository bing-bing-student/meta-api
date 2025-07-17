package tag

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/article"
)

// AdminGetTagList 获取标签列表
func (t *tagService) AdminGetTagList(ctx context.Context) ([]string, error) {
	exist, err := global.RedisSentinel.Exists(global.Context, "tag:articleNum:ZSet").Result()
	if err != nil {
		global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
		return nil, err
	}
	if exist == 1 {
		tagZSet, err := global.RedisSentinel.ZRevRangeWithScores(global.Context, "tag:articleNum:ZSet", 0, -1).Result()
		if err != nil {
			global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
			return nil, err
		}

		for _, label := range tagZSet {
			resp = append(resp, label.Member.(string))
		}
		return resp, nil
	}
	if exist == 0 {
		tagList := make([]article.TagWithArticleCount, 0)
		if err = global.MySqlDB.
			Table("tag").
			Select("tag.name, COUNT(article.id) AS count").
			Joins("JOIN article ON article.tag_id = tag.id").
			Group("tag.id").
			Having("COUNT(article.id) > 0").
			Order("count DESC").
			Find(&tagList).
			Error; err != nil {
			global.Logger.Error("failed to get tags with article", zap.Error(err))
			return nil, err
		}

		if len(tagList) > 0 {
			zAddArgs := make([]redis.Z, len(tagList))
			for i, data := range tagList {
				zAddArgs[i] = redis.Z{
					Score:  float64(data.Count),
					Member: data.Name,
				}
				resp = append(resp, data.Name)
			}

			// 批量写入Redis
			if err = global.RedisSentinel.ZAdd(global.Context, "tag:articleNum:ZSet", zAddArgs...).Err(); err != nil {
				global.Logger.Error("failed to write tag:articleNum:ZSet", zap.Error(err))
				return nil, err
			}
		}
	}
	return resp, nil
}
