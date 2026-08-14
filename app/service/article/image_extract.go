package article

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

var markdownImagePattern = regexp.MustCompile(`!\[[^\]]*]\(\s*(<[^>]+>|[^\s)]+)(?:\s+["'][^"']*["'])?\s*\)`)

type extractedImageRef struct {
	URL     string
	RefType string
	Count   int
}

func extractArticleImageRefs(content string) []extractedImageRef {
	refs := make(map[string]*extractedImageRef)
	add := func(rawURL string, refType string) {
		imageURL := normalizeExtractedImageURL(rawURL)
		if imageURL == "" {
			return
		}
		key := refType + "\x00" + imageURL
		if ref, ok := refs[key]; ok {
			ref.Count++
			return
		}
		refs[key] = &extractedImageRef{
			URL:     imageURL,
			RefType: refType,
			Count:   1,
		}
	}

	for _, match := range markdownImagePattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			add(match[1], "markdown")
		}
	}

	nodes, err := html.ParseFragment(strings.NewReader(content), nil)
	if err == nil {
		var walk func(*html.Node)
		walk = func(node *html.Node) {
			if node.Type == html.ElementNode && strings.EqualFold(node.Data, "img") {
				if src := htmlAttr(node, "src"); src != "" {
					add(src, "html")
				}
				for _, srcsetURL := range imageSrcsetURLs(htmlAttr(node, "srcset")) {
					add(srcsetURL, "html")
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		for _, node := range nodes {
			walk(node)
		}
	}

	result := make([]extractedImageRef, 0, len(refs))
	for _, ref := range refs {
		result = append(result, *ref)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].URL == result[j].URL {
			return result[i].RefType < result[j].RefType
		}
		return result[i].URL < result[j].URL
	})
	return result
}

func normalizeExtractedImageURL(rawURL string) string {
	imageURL := strings.TrimSpace(rawURL)
	imageURL = strings.Trim(imageURL, "<>")
	if imageURL == "" || strings.HasPrefix(strings.ToLower(imageURL), "data:") {
		return ""
	}
	return imageURL
}

func htmlAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func imageSrcsetURLs(srcset string) []string {
	if strings.TrimSpace(srcset) == "" {
		return nil
	}
	parts := strings.Split(srcset, ",")
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) > 0 {
			urls = append(urls, fields[0])
		}
	}
	return urls
}
