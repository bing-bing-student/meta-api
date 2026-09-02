package moderation

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"
)

// 下方嵌入文件由 Unicode Emoji 16.0 与 Unicode CLDR 48.2.1 中文注释生成。
// 数据只在构建维护阶段更新，线上服务运行时不会发起网络请求。
//
//go:generate go run ./internal/genemoji -output emoji_annotations_zh.json.gz
//go:embed emoji_annotations_zh.json.gz
var compressedEmojiAnnotations []byte

// emojiAnnotationPayload 描述嵌入压缩文件中的版本信息及 Emoji 到中文注释的映射。
type emojiAnnotationPayload struct {
	CLDRVersion  string              `json:"cldrVersion"`
	EmojiVersion string              `json:"emojiVersion"`
	Annotations  map[string][]string `json:"annotations"`
}

// emojiTrieNode 是按 UTF-8 字节构建的 Emoji 前缀树节点，用于最长序列匹配。
type emojiTrieNode struct {
	children    map[byte]*emojiTrieNode
	annotations []string
}

// emojiAnnotationIndex 保存 Emoji 前缀树、完整注释映射以及对应的数据版本。
type emojiAnnotationIndex struct {
	root         *emojiTrieNode
	annotations  map[string][]string
	cldrVersion  string
	emojiVersion string
}

// emojiOccurrence 表示文本中一次 Emoji 最长匹配的位置、原始内容和候选中文注释。
type emojiOccurrence struct {
	Text        string
	Start       int
	End         int
	Annotations []string
}

var (
	emojiIndexOnce sync.Once
	emojiIndex     *emojiAnnotationIndex
	emojiIndexErr  error
)

// resolveEmojiAnnotationIndex 延迟加载并全局复用嵌入的 Emoji 注释索引。
// 无输入；返回索引及首次加载错误，后续调用保持相同结果。
func resolveEmojiAnnotationIndex() (*emojiAnnotationIndex, error) {
	emojiIndexOnce.Do(func() {
		emojiIndex, emojiIndexErr = loadEmojiAnnotationIndex(compressedEmojiAnnotations)
	})
	return emojiIndex, emojiIndexErr
}

// loadEmojiAnnotationIndex 解压 compressed，解析注释数据并构建 UTF-8 前缀树。
// 返回可检索索引；压缩、JSON 或空数据异常时返回错误。
func loadEmojiAnnotationIndex(compressed []byte) (*emojiAnnotationIndex, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open embedded emoji annotations: %w", err)
	}
	defer reader.Close()
	var payload emojiAnnotationPayload
	decoder := json.NewDecoder(io.LimitReader(reader, 8<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode embedded emoji annotations: %w", err)
	}
	if len(payload.Annotations) == 0 {
		return nil, fmt.Errorf("embedded emoji annotations are empty")
	}
	index := &emojiAnnotationIndex{
		root:         &emojiTrieNode{},
		annotations:  payload.Annotations,
		cldrVersion:  payload.CLDRVersion,
		emojiVersion: payload.EmojiVersion,
	}
	for emoji, annotations := range payload.Annotations {
		node := index.root
		for _, value := range []byte(emoji) {
			if node.children == nil {
				node.children = make(map[byte]*emojiTrieNode)
			}
			child := node.children[value]
			if child == nil {
				child = &emojiTrieNode{}
				node.children[value] = child
			}
			node = child
		}
		node.annotations = annotations
	}
	return index, nil
}

// find 在 value 中按最长匹配查找所有已收录 Emoji 序列。
// 返回包含字节起止位置和中文注释的出现列表；索引或文本为空时返回 nil。
func (index *emojiAnnotationIndex) find(value string) []emojiOccurrence {
	if index == nil || index.root == nil || value == "" {
		return nil
	}
	result := make([]emojiOccurrence, 0, 2)
	for offset := 0; offset < len(value); {
		node := index.root
		bestEnd := -1
		var best *emojiTrieNode
		for cursor := offset; cursor < len(value); cursor++ {
			if node.children == nil {
				break
			}
			node = node.children[value[cursor]]
			if node == nil {
				break
			}
			if len(node.annotations) > 0 {
				best = node
				bestEnd = cursor + 1
			}
		}
		if best != nil {
			result = append(result, emojiOccurrence{
				Text:        value[offset:bestEnd],
				Start:       offset,
				End:         bestEnd,
				Annotations: best.annotations,
			})
			offset = bestEnd
			continue
		}
		_, size := utf8.DecodeRuneInString(value[offset:])
		if size <= 0 {
			size = 1
		}
		offset += size
	}
	return result
}
