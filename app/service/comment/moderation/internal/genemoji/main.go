package main

import (
	"bufio"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	cldrVersion        = "48.2.1"
	emojiVersion       = "16.0"
	baseAnnotationsURL = "https://raw.githubusercontent.com/unicode-org/cldr-json/48.2.1/cldr-json/cldr-annotations-full/annotations/zh/annotations.json"
	derivedURL         = "https://raw.githubusercontent.com/unicode-org/cldr-json/48.2.1/cldr-json/cldr-annotations-derived-full/annotationsDerived/zh/annotations.json"
	emojiTestURL       = "https://www.unicode.org/Public/emoji/16.0/emoji-test.txt"
)

// annotation 表示 CLDR 为单个 Emoji 提供的搜索关键词和文本转语音名称。
type annotation struct {
	Default []string `json:"default"`
	TTS     []string `json:"tts"`
}

// annotationSection 对应 CLDR 文档中的 Emoji 注释映射区域。
type annotationSection struct {
	Annotations map[string]annotation `json:"annotations"`
}

// annotationDocument 兼容基础注释与派生注释两种 CLDR 文档结构。
type annotationDocument struct {
	Annotations        annotationSection `json:"annotations"`
	AnnotationsDerived annotationSection `json:"annotationsDerived"`
}

// generatedData 是写入审核服务嵌入文件的精简版本信息和中文注释数据。
type generatedData struct {
	CLDRVersion  string              `json:"cldrVersion"`
	EmojiVersion string              `json:"emojiVersion"`
	Annotations  map[string][]string `json:"annotations"`
}

// main 解析输出参数并运行 Emoji 注释生成任务；生成失败时输出错误并以非零状态退出。
func main() {
	output := flag.String("output", "emoji_annotations_zh.json.gz", "generated gzip output")
	flag.Parse()
	if err := run(*output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "generate emoji annotations: %v\n", err)
		os.Exit(1)
	}
}

// run 下载指定版本的 CLDR 和 Emoji 序列数据，合并中文注释并写入 output。
// 输入 output 是 gzip 产物路径；返回下载、解析、覆盖率校验或写入错误。
func run(output string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	annotations := make(map[string]annotation)
	for _, source := range []string{baseAnnotationsURL, derivedURL} {
		body, err := fetch(client, source)
		if err != nil {
			return err
		}
		var document annotationDocument
		if err := json.Unmarshal(body, &document); err != nil {
			return fmt.Errorf("decode %s: %w", source, err)
		}
		for key, value := range document.Annotations.Annotations {
			annotations[key] = mergeAnnotation(annotations[key], value)
		}
		for key, value := range document.AnnotationsDerived.Annotations {
			annotations[key] = mergeAnnotation(annotations[key], value)
		}
	}

	emojiTest, err := fetch(client, emojiTestURL)
	if err != nil {
		return err
	}
	sequences, err := parseEmojiSequences(string(emojiTest))
	if err != nil {
		return err
	}
	generated := generatedData{
		CLDRVersion:  cldrVersion,
		EmojiVersion: emojiVersion,
		Annotations:  make(map[string][]string, len(sequences)),
	}
	for _, sequence := range sequences {
		values := annotationValues(annotations[sequence])
		if len(values) == 0 {
			values = annotationValues(annotations[stripVariationSelectors(sequence)])
		}
		if len(values) == 0 {
			continue
		}
		generated.Annotations[sequence] = values
	}
	if len(generated.Annotations) < 3000 {
		return fmt.Errorf("only %d emoji sequences have annotations", len(generated.Annotations))
	}
	return writeGenerated(output, generated)
}

// fetch 使用 client 下载 source，并限制响应体最大为 8 MiB。
// 返回响应字节；网络、非成功状态码或读取失败时返回错误。
func fetch(client *http.Client, source string) ([]byte, error) {
	response, err := client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: status %s", source, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	return body, nil
}

// mergeAnnotation 合并 left 与 right 的 TTS 名称和默认关键词并去重；返回新注释。
func mergeAnnotation(left, right annotation) annotation {
	return annotation{
		TTS:     appendUnique(left.TTS, right.TTS...),
		Default: appendUnique(left.Default, right.Default...),
	}
}

// annotationValues 将 value 的 TTS 名称置于默认关键词之前并去重，返回可嵌入词项。
func annotationValues(value annotation) []string {
	result := appendUnique(nil, value.TTS...)
	return appendUnique(result, value.Default...)
}

// appendUnique 将 additions 清理后追加到 values，并保持首次出现顺序去重；返回合并切片。
func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

// parseEmojiSequences 从 emoji-test.txt 内容 value 中解析并排序所有唯一 Emoji 序列。
// 返回序列集合；码点解码或扫描失败时返回错误。
func parseEmojiSequences(value string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, 5000)
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, ";", 2)
		if len(fields) != 2 {
			continue
		}
		sequence, err := decodeCodePoints(strings.Fields(strings.TrimSpace(fields[0])))
		if err != nil {
			return nil, fmt.Errorf("decode emoji sequence %q: %w", fields[0], err)
		}
		if _, exists := seen[sequence]; exists {
			continue
		}
		seen[sequence] = struct{}{}
		result = append(result, sequence)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan emoji-test.txt: %w", err)
	}
	sort.Strings(result)
	return result, nil
}

// decodeCodePoints 将十六进制 Unicode 码点 values 解码为字符串。
// 返回 Emoji 序列；空输入、非法十六进制或无效码点会返回错误。
func decodeCodePoints(values []string) (string, error) {
	if len(values) == 0 {
		return "", errors.New("empty code point sequence")
	}
	var builder strings.Builder
	for _, value := range values {
		encoded, err := hex.DecodeString(leftPadHex(value))
		if err != nil {
			return "", err
		}
		var codePoint rune
		for _, item := range encoded {
			codePoint = codePoint<<8 | rune(item)
		}
		if !utf8.ValidRune(codePoint) {
			return "", fmt.Errorf("invalid code point %s", value)
		}
		builder.WriteRune(codePoint)
	}
	return builder.String(), nil
}

// leftPadHex 为奇数长度十六进制字符串 value 补一个前导零，并返回偶数长度结果。
func leftPadHex(value string) string {
	if len(value)%2 != 0 {
		return "0" + value
	}
	return value
}

// stripVariationSelectors 移除 value 中的文本和 Emoji 变体选择符，返回基础序列。
func stripVariationSelectors(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\ufe0e' || r == '\ufe0f' {
			return -1
		}
		return r
	}, value)
}

// writeGenerated 将 value 以时间戳固定的 gzip JSON 写入 output，确保生成结果可复现。
// 返回文件创建、编码、压缩关闭或文件关闭错误。
func writeGenerated(output string, value generatedData) error {
	file, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create %s: %w", output, err)
	}
	defer file.Close()
	writer := gzip.NewWriter(file)
	writer.Header.ModTime = time.Unix(0, 0)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_ = writer.Close()
		return fmt.Errorf("encode %s: %w", output, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close gzip %s: %w", output, err)
	}
	return file.Close()
}
