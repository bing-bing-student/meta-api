package sitedynamic

import (
	"context"
	"errors"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/types"
)

func (s *siteDynamicService) UserGetSiteDynamicList(ctx context.Context) (*types.UserGetSiteDynamicListResponse, error) {
	key := cachekey.SiteDynamicPublishedList().String()
	response := &types.UserGetSiteDynamicListResponse{}
	if s.redis != nil {
		rows, err := s.getPublishedFromCache(ctx, key)
		if err == nil && rows != nil {
			response.Rows = rows
			response.Total = len(rows)
			return response, nil
		}
		if err != nil {
			s.logger.Warn("failed to get site dynamics from redis", zap.Error(err))
		}
	}

	rows, err := s.model.ListPublishedSiteDynamics(ctx)
	if err != nil {
		s.logger.Error("failed to list published site dynamics", zap.Error(err))
		return nil, err
	}
	items := make([]types.UserSiteDynamicItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUserSiteDynamicItem(row))
	}
	if s.redis != nil {
		if err = s.setPublishedCache(ctx, key, items); err != nil {
			s.logger.Warn("failed to set site dynamics to redis", zap.Error(err))
		}
	}
	response.Rows = items
	response.Total = len(items)
	return response, nil
}

func (s *siteDynamicService) getPublishedFromCache(ctx context.Context, key string) ([]types.UserSiteDynamicItem, error) {
	value, err := s.redis.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	items := make([]types.UserSiteDynamicItem, 0)
	if err = sonic.Unmarshal([]byte(value), &items); err != nil {
		return nil, fmt.Errorf("unmarshal site dynamic cache: %w", err)
	}
	return items, nil
}

func (s *siteDynamicService) setPublishedCache(ctx context.Context, key string,
	items []types.UserSiteDynamicItem) error {
	value, err := sonic.Marshal(items)
	if err != nil {
		return fmt.Errorf("marshal site dynamic cache: %w", err)
	}
	// 站点动态由后台变更主动删除缓存，不需要基于时间过期。
	return s.redis.Set(ctx, key, value, 0).Err()
}
