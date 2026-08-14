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
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/app/model/article"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/pkg/cos"
)

var errArticleImageInUse = errors.New("article image is still referenced")

type scannedImageRef struct {
	objectKey string
	refType   string
	count     int
	articleID uint64
}

type scanImageDraft struct {
	image    article.ArticleImage
	refCount int
}

func (a *articleService) AdminScanArticleImages(ctx context.Context) (*types.AdminScanArticleImagesResponse, error) {
	now := time.Now()
	objects, err := a.imageStore.List(ctx)
	if err != nil {
		if errors.Is(err, cos.ErrDisabled) {
			return nil, fmt.Errorf("article image storage is not configured: %w", err)
		}
		return nil, err
	}

	sources, err := a.articleModel.ListArticleImageSources(ctx)
	if err != nil {
		return nil, err
	}

	imageDrafts := make(map[string]*scanImageDraft, len(objects))
	for _, object := range objects {
		imageDrafts[object.Key] = &scanImageDraft{
			image: article.ArticleImage{
				ObjectKey:          object.Key,
				URL:                object.URL,
				ImageName:          object.FileName,
				Mime:               object.Mime,
				Size:               object.Size,
				ETag:               object.ETag,
				Status:             article.ImageStatusUnused,
				Source:             article.ImageSourceCOS,
				LastSeenTime:       &now,
				ObjectModifiedTime: object.LastModifiedTime,
				CreateTime:         now,
				UpdateTime:         now,
			},
		}
	}

	referenceDrafts := make([]scannedImageRef, 0)
	for _, source := range sources {
		for _, ref := range extractArticleImageRefs(source.Content) {
			objectKey, ok := a.imageStore.ObjectKeyFromPublicURL(ref.URL)
			if !ok {
				continue
			}
			draft, exists := imageDrafts[objectKey]
			if !exists {
				draft = &scanImageDraft{
					image: article.ArticleImage{
						ObjectKey:    objectKey,
						URL:          ref.URL,
						ImageName:    path.Base(objectKey),
						Mime:         mime.TypeByExtension(path.Ext(objectKey)),
						Status:       article.ImageStatusMissing,
						Source:       article.ImageSourceScan,
						LastSeenTime: &now,
						CreateTime:   now,
						UpdateTime:   now,
					},
				}
				imageDrafts[objectKey] = draft
			}
			draft.refCount += ref.Count
			referenceDrafts = append(referenceDrafts, scannedImageRef{
				objectKey: objectKey,
				refType:   ref.RefType,
				count:     ref.Count,
				articleID: source.ID,
			})
		}
	}

	objectKeys := sortedImageObjectKeys(imageDrafts)
	existing, err := a.articleModel.FindArticleImagesByObjectKeys(ctx, objectKeys)
	if err != nil {
		return nil, err
	}

	images := make([]article.ArticleImage, 0, len(objectKeys))
	imageIDs := make(map[string]uint64, len(objectKeys))
	stats := &types.AdminScanArticleImagesResponse{ArticleTotal: len(sources)}
	for _, objectKey := range objectKeys {
		draft := imageDrafts[objectKey]
		if draft.refCount > 0 && draft.image.Status != article.ImageStatusMissing {
			draft.image.Status = article.ImageStatusUsed
		}
		switch draft.image.Status {
		case article.ImageStatusUsed:
			stats.UsedTotal++
		case article.ImageStatusMissing:
			stats.MissingTotal++
		default:
			stats.UnusedTotal++
		}

		if old, ok := existing[objectKey]; ok {
			draft.image.ID = old.ID
			draft.image.CreateTime = old.CreateTime
			if draft.image.Source == article.ImageSourceCOS && old.Source == article.ImageSourceUpload {
				draft.image.Source = article.ImageSourceUpload
			}
		} else {
			id, err := a.idGenerator.NextID()
			if err != nil {
				return nil, fmt.Errorf("generate article image id: %w", err)
			}
			draft.image.ID = id
		}
		imageIDs[objectKey] = draft.image.ID
		images = append(images, draft.image)
	}

	references := make([]article.ArticleImageReference, 0, len(referenceDrafts))
	for _, draft := range referenceDrafts {
		imageID, ok := imageIDs[draft.objectKey]
		if !ok {
			continue
		}
		id, err := a.idGenerator.NextID()
		if err != nil {
			return nil, fmt.Errorf("generate article image reference id: %w", err)
		}
		references = append(references, article.ArticleImageReference{
			ID:         id,
			ImageID:    imageID,
			ArticleID:  draft.articleID,
			RefType:    draft.refType,
			RefCount:   draft.count,
			CreateTime: now,
			UpdateTime: now,
		})
		stats.ReferenceTotal += draft.count
	}
	stats.ImageTotal = len(images)

	if err = a.articleModel.SyncArticleImages(ctx, images, references); err != nil {
		return nil, err
	}
	return stats, nil
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

	if image.Status != article.ImageStatusMissing {
		if err = a.imageStore.Delete(ctx, image.ObjectKey); err != nil {
			if errors.Is(err, cos.ErrDisabled) {
				return fmt.Errorf("article image storage is not configured: %w", err)
			}
			return err
		}
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

func sortedImageObjectKeys(drafts map[string]*scanImageDraft) []string {
	keys := make([]string, 0, len(drafts))
	for key := range drafts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func articleImageListItem(record article.ArticleImageListRecord) types.AdminArticleImageListItem {
	return types.AdminArticleImageListItem{
		ID:                 strconv.FormatUint(record.ID, 10),
		URL:                record.URL,
		ImageName:          record.ImageName,
		Mime:               record.Mime,
		Size:               record.Size,
		ETag:               record.ETag,
		Status:             record.Status,
		Source:             record.Source,
		RefCount:           record.RefCount,
		LastSeenTime:       formatOptionalMinute(record.LastSeenTime),
		ObjectModifiedTime: formatOptionalMinute(record.ObjectModifiedTime),
		CreateTime:         record.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:         record.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func articleImageItemFromModel(image *article.ArticleImage, refCount int) types.AdminArticleImageListItem {
	return types.AdminArticleImageListItem{
		ID:                 strconv.FormatUint(image.ID, 10),
		URL:                image.URL,
		ImageName:          image.ImageName,
		Mime:               image.Mime,
		Size:               image.Size,
		ETag:               image.ETag,
		Status:             image.Status,
		Source:             image.Source,
		RefCount:           refCount,
		LastSeenTime:       formatOptionalMinute(image.LastSeenTime),
		ObjectModifiedTime: formatOptionalMinute(image.ObjectModifiedTime),
		CreateTime:         image.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:         image.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
}

func formatOptionalMinute(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format(constants.TimeLayoutToMinute)
}

func IsArticleImageInUseError(err error) bool {
	return errors.Is(err, errArticleImageInUse)
}

func IsArticleImageNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
