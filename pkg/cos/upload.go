package cos

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

func (c *Client) Upload(ctx context.Context, objectName string, content []byte, contentType string) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrDisabled
	}
	objectKey := c.objectKey(objectName)
	opt := &tencentcos.ObjectPutOptions{
		ObjectPutHeaderOptions: &tencentcos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			ContentLength: int64(len(content)),
		},
	}
	if _, err := c.client.Object.Put(ctx, objectKey, bytes.NewReader(content), opt); err != nil {
		return "", fmt.Errorf("upload article image to COS: %w", err)
	}
	publicName := objectKey
	if c.hasCustomPublicBase {
		publicName = strings.TrimLeft(path.Clean("/"+objectName), "/")
	}
	return c.publicURL(publicName), nil
}
