package cos

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

const listMaxKeys = 1000

type Object struct {
	Key              string
	URL              string
	FileName         string
	Mime             string
	Size             int64
	ETag             string
	LastModifiedTime *time.Time
}

func (c *Client) List(ctx context.Context) ([]Object, error) {
	if c == nil || c.client == nil {
		return nil, ErrDisabled
	}

	prefix := c.directory
	if prefix != "" {
		prefix += "/"
	}

	objects := make([]Object, 0)
	marker := ""
	for {
		result, _, err := c.client.Bucket.Get(ctx, &tencentcos.BucketGetOptions{
			Prefix:  prefix,
			Marker:  marker,
			MaxKeys: listMaxKeys,
		})
		if err != nil {
			return nil, fmt.Errorf("list article images from COS: %w", err)
		}

		for _, item := range result.Contents {
			key := strings.TrimSpace(item.Key)
			if key == "" || strings.HasSuffix(key, "/") {
				continue
			}
			objects = append(objects, Object{
				Key:              key,
				URL:              c.PublicURL(key),
				FileName:         path.Base(key),
				Mime:             mime.TypeByExtension(path.Ext(key)),
				Size:             item.Size,
				ETag:             strings.Trim(item.ETag, `"`),
				LastModifiedTime: parseObjectLastModified(item.LastModified),
			})
		}

		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
		if marker == "" && len(result.Contents) > 0 {
			marker = result.Contents[len(result.Contents)-1].Key
		}
		if marker == "" {
			return nil, fmt.Errorf("list article images from COS: missing next marker")
		}
	}

	return objects, nil
}

func (c *Client) Delete(ctx context.Context, objectKey string) error {
	if c == nil || c.client == nil {
		return ErrDisabled
	}
	key := strings.TrimLeft(path.Clean("/"+objectKey), "/")
	if key == "" || key == "." {
		return fmt.Errorf("empty object key")
	}
	if _, err := c.client.Object.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete article image from COS: %w", err)
	}
	return nil
}

func (c *Client) ObjectKey(objectName string) string {
	return c.objectKey(objectName)
}

func (c *Client) PublicURL(objectKey string) string {
	publicName := strings.TrimLeft(path.Clean("/"+objectKey), "/")
	if c.hasCustomPublicBase && c.directory != "" {
		publicName = strings.TrimPrefix(publicName, c.directory+"/")
	}
	return c.publicURL(publicName)
}

func (c *Client) ObjectKeyFromPublicURL(rawURL string) (string, bool) {
	if c == nil {
		return "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	base, err := url.Parse(c.publicBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", false
	}
	if !strings.EqualFold(parsed.Host, base.Host) {
		return "", false
	}

	basePath := strings.TrimRight(base.EscapedPath(), "/")
	imagePath := parsed.EscapedPath()
	if basePath != "" {
		if imagePath != basePath && !strings.HasPrefix(imagePath, basePath+"/") {
			return "", false
		}
		imagePath = strings.TrimPrefix(imagePath, basePath)
	}

	relativePath, err := url.PathUnescape(strings.TrimLeft(imagePath, "/"))
	if err != nil {
		return "", false
	}
	if relativePath == "" {
		return "", false
	}
	return c.objectKey(relativePath), true
}

func parseObjectLastModified(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}
	return nil
}
