package link

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/types"
)

// UserGetLinkList 获取链接列表
func (l *linkService) UserGetLinkList(ctx context.Context) (*types.UserGetLinkListResponse, error) {
	key := "link:ZSet"
	response := &types.UserGetLinkListResponse{}

	// 缓存不存在
	if exists := l.redis.Exists(ctx, key).Val(); exists == 0 {
		// 查询所有友链数据
		linkList, err := l.model.GetLinkList(ctx)
		if err != nil {
			l.logger.Error("failed to get link list", zap.Error(err))
			return nil, fmt.Errorf("failed to get link list, err: %w", err)
		}

		for _, linkItem := range linkList {
			// 设置缓存
			item := types.LinkItem{
				ID:   strconv.Itoa(int(linkItem.ID)),
				Name: linkItem.Name,
				URL:  linkItem.URL,
			}
			member, err := sonic.Marshal(item)
			if err != nil {
				l.logger.Error("failed to marshal member", zap.Error(err))
				return nil, err
			}
			l.redis.ZAdd(ctx, key, redis.Z{
				Score:  float64(linkItem.UpdateTime.UnixNano() / int64(time.Millisecond)),
				Member: member,
			})

			response.Rows = append(response.Rows, item)
		}
		response.Total = len(response.Rows)
		return response, nil
	}

	// 缓存存在
	linkList, err := l.redis.ZRangeWithScores(ctx, key, 0, -1).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		l.logger.Error("failed to get linkList from redis", zap.Error(err))
		return nil, fmt.Errorf("failed to get linkList from redis, err: %w", err)
	}
	for _, linkItem := range linkList {
		member, ok := linkItem.Member.(string)
		if !ok {
			l.logger.Error("failed to convert member to string", zap.Error(err))
			return nil, fmt.Errorf("failed to convert member to string, err: %w", err)
		}
		item := types.LinkItem{}
		if err = sonic.Unmarshal([]byte(member), &item); err != nil {
			l.logger.Error("failed to unmarshal member", zap.Error(err))
			return nil, fmt.Errorf("failed to unmarshal member, err: %w", err)
		}

		response.Rows = append(response.Rows, item)
	}

	response.Total = len(response.Rows)
	return response, nil
}
