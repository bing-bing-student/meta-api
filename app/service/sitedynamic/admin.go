package sitedynamic

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	siteDynamicModel "meta-api/app/model/sitedynamic"
	"meta-api/common/cachekey"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

const siteDynamicContentMaxRunes = 50

func (s *siteDynamicService) AdminGetSiteDynamicList(ctx context.Context) (*types.AdminGetSiteDynamicListResponse, error) {
	rows, err := s.model.ListSiteDynamics(ctx)
	if err != nil {
		s.logger.Error("failed to list site dynamics", zap.Error(err))
		return nil, err
	}
	items := make([]types.AdminSiteDynamicItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAdminSiteDynamicItem(row))
	}
	return &types.AdminGetSiteDynamicListResponse{
		Rows:  items,
		Total: len(items),
	}, nil
}

func (s *siteDynamicService) AdminAddSiteDynamic(ctx context.Context, request *types.AdminAddSiteDynamicRequest) error {
	content, err := normalizeSiteDynamicContent(request.Content)
	if err != nil {
		return err
	}
	now, err := siteDynamicNow()
	if err != nil {
		return err
	}
	id, err := s.idGenerator.NextID()
	if err != nil {
		s.logger.Error("failed to generate site dynamic id", zap.Error(err))
		return fmt.Errorf("generate site dynamic id: %w", err)
	}
	item := &siteDynamicModel.SiteDynamic{
		ID:                  id,
		Content:             content,
		Status:              request.Status,
		DeprecatedEventTime: now,
		CreateTime:          now,
		UpdateTime:          now,
	}
	if err = s.model.CreateSiteDynamic(ctx, item); err != nil {
		s.logger.Error("failed to create site dynamic", zap.Error(err))
		return err
	}
	return s.clearPublishedCache(ctx)
}

func (s *siteDynamicService) AdminUpdateSiteDynamic(ctx context.Context, request *types.AdminUpdateSiteDynamicRequest) error {
	id, err := idutil.ParseID("siteDynamicID", request.ID)
	if err != nil {
		return err
	}
	content, err := normalizeSiteDynamicContent(request.Content)
	if err != nil {
		return err
	}
	now, err := siteDynamicNow()
	if err != nil {
		return err
	}
	item := &siteDynamicModel.SiteDynamic{
		ID:         id,
		Content:    content,
		Status:     request.Status,
		UpdateTime: now,
	}
	if err = s.model.UpdateSiteDynamic(ctx, item); err != nil {
		s.logger.Error("failed to update site dynamic", zap.Error(err))
		return err
	}
	return s.clearPublishedCache(ctx)
}

func normalizeSiteDynamicContent(value string) (string, error) {
	content := strings.TrimSpace(value)
	if content == "" {
		return "", fmt.Errorf("site dynamic content is required")
	}
	if utf8.RuneCountInString(content) > siteDynamicContentMaxRunes {
		return "", fmt.Errorf("site dynamic content exceeds %d characters", siteDynamicContentMaxRunes)
	}
	return content, nil
}

func (s *siteDynamicService) AdminReorderSiteDynamics(ctx context.Context, request *types.AdminReorderSiteDynamicRequest) error {
	ids := make([]uint64, 0, len(request.IDs))
	seen := make(map[uint64]struct{}, len(request.IDs))
	for _, rawID := range request.IDs {
		id, err := idutil.ParseID("siteDynamicID", rawID)
		if err != nil {
			return err
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicated site dynamic id: %s", rawID)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	now, err := siteDynamicNow()
	if err != nil {
		return err
	}
	if err = s.model.ReorderSiteDynamics(ctx, ids, now); err != nil {
		s.logger.Error("failed to reorder site dynamics", zap.Error(err))
		return err
	}
	return s.clearPublishedCache(ctx)
}

func (s *siteDynamicService) AdminDeleteSiteDynamic(ctx context.Context, request *types.AdminDeleteSiteDynamicRequest) error {
	id, err := idutil.ParseID("siteDynamicID", request.ID)
	if err != nil {
		return err
	}
	if err = s.model.DeleteSiteDynamic(ctx, id); err != nil {
		s.logger.Error("failed to delete site dynamic", zap.Error(err))
		return err
	}
	return s.clearPublishedCache(ctx)
}

func (s *siteDynamicService) clearPublishedCache(ctx context.Context) error {
	if s.redis == nil {
		return nil
	}
	key := cachekey.SiteDynamicPublishedList().String()
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		s.logger.Error("failed to clear site dynamic cache", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("clear site dynamic cache: %w", err)
	}
	return nil
}
