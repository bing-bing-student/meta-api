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

// The generated artifact is derived from Unicode Emoji 16.0 and the zh
// annotations in Unicode CLDR 48.2.1. It is generated at build-maintenance
// time and never fetched by the running service.
//
//go:generate go run ./internal/genemoji -output emoji_annotations_zh.json.gz
//go:embed emoji_annotations_zh.json.gz
var compressedEmojiAnnotations []byte

type emojiAnnotationPayload struct {
	CLDRVersion  string              `json:"cldrVersion"`
	EmojiVersion string              `json:"emojiVersion"`
	Annotations  map[string][]string `json:"annotations"`
}

type emojiTrieNode struct {
	children    map[byte]*emojiTrieNode
	annotations []string
}

type emojiAnnotationIndex struct {
	root         *emojiTrieNode
	annotations  map[string][]string
	cldrVersion  string
	emojiVersion string
}

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

func resolveEmojiAnnotationIndex() (*emojiAnnotationIndex, error) {
	emojiIndexOnce.Do(func() {
		emojiIndex, emojiIndexErr = loadEmojiAnnotationIndex(compressedEmojiAnnotations)
	})
	return emojiIndex, emojiIndexErr
}

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
