package cos

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
)

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
