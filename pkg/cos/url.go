package cos

import (
	"net/url"
	"path"
	"strings"
)

func (c *Client) objectKey(objectName string) string {
	name := strings.TrimLeft(path.Clean("/"+objectName), "/")
	if c.directory == "" {
		return name
	}
	return path.Join(c.directory, name)
}

func (c *Client) publicURL(objectKey string) string {
	if parsed, err := url.Parse(c.publicBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Path = path.Join(parsed.Path, objectKey)
		return parsed.String()
	}
	return strings.TrimRight(c.publicBaseURL, "/") + "/" + strings.TrimLeft(objectKey, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
