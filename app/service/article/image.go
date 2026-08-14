package article

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/app/model/article"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/pkg/cos"
)

var errArticleImageInUse = errors.New("article image is still referenced")

type articleImageRefDraft struct {
	objectKey string
	refType   string
	count     int
}

func (a *articleService) syncPublishedArticleImageReferences(ctx context.Context, articleID uint64,
	content string) error {
	refDrafts := make(map[string]*articleImageRefDraft)
	objectKeys := make(map[string]struct{})
	for _, ref := range extractArticleImageRefs(content) {
		objectKey, ok := a.imageStore.ObjectKeyFromPublicURL(ref.URL)
		if !ok {
			continue
		}
		objectKeys[objectKey] = struct{}{}
		key := objectKey + "\x00" + ref.RefType
		if draft, exists := refDrafts[key]; exists {
			draft.count += ref.Count
			continue
		}
		refDrafts[key] = &articleImageRefDraft{
			objectKey: objectKey,
			refType:   ref.RefType,
			count:     ref.Count,
		}
	}

	keys := make([]string, 0, len(objectKeys))
	for objectKey := range objectKeys {
		keys = append(keys, objectKey)
	}
	sort.Strings(keys)

	existing, err := a.articleModel.FindArticleImagesByObjectKeys(ctx, keys)
	if err != nil {
		return err
	}

	now := articleNow()
	images := make([]article.ArticleImage, 0)
	imageIDs := make(map[string]uint64, len(keys))
	for _, objectKey := range keys {
		if image, ok := existing[objectKey]; ok {
			imageIDs[objectKey] = image.ID
			continue
		}

		id, err := a.idGenerator.NextID()
		if err != nil {
			return fmt.Errorf("generate article image id: %w", err)
		}
		imageIDs[objectKey] = id
		images = append(images, article.ArticleImage{
			ID:         id,
			ObjectKey:  objectKey,
			URL:        a.imageStore.PublicURL(objectKey),
			ImageName:  path.Base(objectKey),
			Mime:       mime.TypeByExtension(path.Ext(objectKey)),
			Status:     article.ImageStatusUsed,
			CreateTime: now,
			UpdateTime: now,
		})
	}

	refKeys := make([]string, 0, len(refDrafts))
	for key := range refDrafts {
		refKeys = append(refKeys, key)
	}
	sort.Strings(refKeys)

	references := make([]article.ArticleImageReference, 0, len(refKeys))
	for _, key := range refKeys {
		draft := refDrafts[key]
		imageID, ok := imageIDs[draft.objectKey]
		if !ok {
			continue
		}
		id, err := a.idGenerator.NextID()
		if err != nil {
			return fmt.Errorf("generate article image reference id: %w", err)
		}
		references = append(references, article.ArticleImageReference{
			ID:         id,
			ImageID:    imageID,
			ArticleID:  articleID,
			RefType:    draft.refType,
			RefCount:   draft.count,
			CreateTime: now,
			UpdateTime: now,
		})
	}

	return a.articleModel.SyncArticleImageReferences(ctx, articleID, images, references)
}

func (a *articleService) AdminGetArticleImageList(ctx context.Context,
	request *types.AdminGetArticleImageListRequest) (*types.AdminGetArticleImageListResponse, error) {
	offset := (request.Page - 1) * request.PageSize
	records, total, err := a.articleModel.ListArticleImages(ctx, article.ArticleImageQuery{
		Status:  strings.TrimSpace(request.Status),
		Keyword: strings.TrimSpace(request.Keyword),
		Offset:  offset,
		Limit:   request.PageSize,
	})
	if err != nil {
		return nil, err
	}

	rows := make([]types.AdminArticleImageListItem, 0, len(records))
	for _, record := range records {
		rows = append(rows, articleImageListItem(record))
	}
	return &types.AdminGetArticleImageListResponse{
		Rows:  rows,
		Total: int(total),
	}, nil
}

func (a *articleService) AdminGetArticleImageDetail(ctx context.Context,
	request *types.AdminGetArticleImageDetailRequest) (*types.AdminGetArticleImageDetailResponse, error) {
	id, err := idutil.ParseID("articleImageID", request.ID)
	if err != nil {
		return nil, err
	}
	image, err := a.articleModel.GetArticleImageByID(ctx, id)
	if err != nil {
		return nil, err
	}
	references, err := a.articleModel.ListArticleImageReferences(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]types.AdminArticleImageReferenceItem, 0, len(references))
	refCount := 0
	for _, ref := range references {
		refCount += ref.RefCount
		items = append(items, types.AdminArticleImageReferenceItem{
			ID:           strconv.FormatUint(ref.ID, 10),
			ArticleID:    strconv.FormatUint(ref.ArticleID, 10),
			ArticleTitle: ref.ArticleTitle,
		})
	}

	return &types.AdminGetArticleImageDetailResponse{
		Image:      articleImageItemFromModel(image, refCount),
		References: items,
	}, nil
}

func (a *articleService) AdminDeleteArticleImage(ctx context.Context,
	request *types.AdminDeleteArticleImageRequest) error {
	id, err := idutil.ParseID("articleImageID", request.ID)
	if err != nil {
		return err
	}
	image, err := a.articleModel.GetArticleImageByID(ctx, id)
	if err != nil {
		return err
	}
	refCount, err := a.articleModel.CountArticleImageReferences(ctx, id)
	if err != nil {
		return err
	}
	if refCount > 0 && !request.Force {
		return errArticleImageInUse
	}

	if err = a.imageStore.Delete(ctx, image.ObjectKey); err != nil {
		if errors.Is(err, cos.ErrDisabled) {
			return fmt.Errorf("article image storage is not configured: %w", err)
		}
		return err
	}
	if err = a.articleModel.DeleteArticleImage(ctx, id); err != nil {
		return err
	}
	a.logger.Info("article image deleted",
		zap.String("image_id", request.ID),
		zap.String("object_key", image.ObjectKey),
		zap.Bool("force", request.Force))
	return nil
}

func articleImageListItem(record article.ArticleImageListRecord) types.AdminArticleImageListItem {
	return types.AdminArticleImageListItem{
		ID:         strconv.FormatUint(record.ID, 10),
		URL:        record.URL,
		ImageName:  record.ImageName,
		Mime:       record.Mime,
		Size:       record.Size,
		ETag:       record.ETag,
		Status:     record.Status,
		RefCount:   record.RefCount,
		CreateTime: record.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime: record.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func articleImageItemFromModel(image *article.ArticleImage, refCount int) types.AdminArticleImageListItem {
	return types.AdminArticleImageListItem{
		ID:         strconv.FormatUint(image.ID, 10),
		URL:        image.URL,
		ImageName:  image.ImageName,
		Mime:       image.Mime,
		Size:       image.Size,
		ETag:       image.ETag,
		Status:     image.Status,
		RefCount:   refCount,
		CreateTime: image.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime: image.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func IsArticleImageInUseError(err error) bool {
	return errors.Is(err, errArticleImageInUse)
}

func IsArticleImageNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
