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

	"meta-api/app/model/link"
	"meta-api/common/types"
)

// AdminGetLinkList 获取友链列表
func (l *linkService) AdminGetLinkList(ctx context.Context) (*types.AdminGetLinkListResponse, error) {
	response := &types.AdminGetLinkListResponse{}
	key := "link:ZSet"
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

// AdminAddLink 添加友链
func (l *linkService) AdminAddLink(ctx context.Context, request *types.AdminAddLinkRequest) error {
	// 添加友链
	id, err := l.idGenerator.NextID()
	if err != nil {
		l.logger.Error("failed to generate id", zap.Error(err))
		return fmt.Errorf("failed to generate id, error: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		l.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location, error: %w", err)
	}
	nowTime := time.Now().In(loc)
	linkInfo := &link.Link{
		ID:         id,
		Name:       request.Name,
		URL:        request.URL,
		CreateTime: nowTime,
		UpdateTime: nowTime,
	}
	if err = l.model.CreateLink(ctx, linkInfo); err != nil {
		l.logger.Error("failed to create link", zap.Error(err))
		return fmt.Errorf("failed to create link, error: %w", err)
	}

	// 添加缓存
	key := "link:ZSet"
	item := types.LinkItem{
		ID:   strconv.Itoa(int(linkInfo.ID)),
		Name: linkInfo.Name,
		URL:  linkInfo.URL,
	}
	member, err := sonic.Marshal(item)
	if err != nil {
		l.logger.Error("failed to marshal LinkItem member", zap.Error(err))
		return fmt.Errorf("failed to marshal LinkItem member, err: %w", err)
	}
	if err = l.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(linkInfo.UpdateTime.UnixNano() / int64(time.Millisecond)),
		Member: member,
	}).Err(); err != nil {
		l.logger.Error("failed to add LinkItem member to Redis", zap.Error(err))
		return fmt.Errorf("failed to add LinkItem member to Redis, err: %w", err)
	}

	return nil
}

// AdminUpdateLink 修改友链
func (l *linkService) AdminUpdateLink(ctx context.Context, request *types.AdminUpdateLinkRequest) error {
	// 更新MySQL数据
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		l.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location, error: %w", err)
	}
	id, err := strconv.ParseUint(request.ID, 10, 64)
	if err != nil {
		l.logger.Error("parse uint64 error", zap.Error(err))
		return err
	}
	linkInfo := &link.Link{
		ID:         id,
		Name:       request.Name,
		URL:        request.URL,
		UpdateTime: time.Now().In(loc),
	}
	if err = l.model.UpdateLink(ctx, linkInfo); err != nil {
		l.logger.Error("failed to update link", zap.Error(err))
		return fmt.Errorf("failed to update link, error: %w", err)
	}

	// 删除缓存
	key := "link:ZSet"
	if err = l.redis.Del(ctx, key).Err(); err != nil {
		l.logger.Error("failed to delete link list from Redis", zap.Error(err))
		return fmt.Errorf("failed to delete link list from Redis, err: %w", err)
	}

	return nil
}

// AdminDeleteLink 删除友链
func (l *linkService) AdminDeleteLink(ctx context.Context, request *types.AdminDeleteLinkRequest) error {
	// 删除MySQL数据
	id, err := strconv.ParseUint(request.ID, 10, 64)
	if err != nil {
		l.logger.Error("parse uint64 error", zap.Error(err))
		return fmt.Errorf("parse uint64 error: %w", err)
	}
	if err = l.model.DeleteLink(ctx, id); err != nil {
		l.logger.Error("failed to delete link", zap.Error(err))
		return fmt.Errorf("failed to delete link, error: %w", err)
	}

	// 删除缓存
	key := "link:ZSet"
	if err = l.redis.Del(ctx, key).Err(); err != nil {
		l.logger.Error("failed to delete link list from Redis", zap.Error(err))
		return fmt.Errorf("failed to delete link list from Redis, err: %w", err)
	}

	return nil
}
