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

type annotation struct {
	Default []string `json:"default"`
	TTS     []string `json:"tts"`
}

type annotationSection struct {
	Annotations map[string]annotation `json:"annotations"`
}

type annotationDocument struct {
	Annotations        annotationSection `json:"annotations"`
	AnnotationsDerived annotationSection `json:"annotationsDerived"`
}

type generatedData struct {
	CLDRVersion  string              `json:"cldrVersion"`
	EmojiVersion string              `json:"emojiVersion"`
	Annotations  map[string][]string `json:"annotations"`
}

func main() {
	output := flag.String("output", "emoji_annotations_zh.json.gz", "generated gzip output")
	flag.Parse()
	if err := run(*output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "generate emoji annotations: %v\n", err)
		os.Exit(1)
	}
}

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

func mergeAnnotation(left, right annotation) annotation {
	return annotation{
		TTS:     appendUnique(left.TTS, right.TTS...),
		Default: appendUnique(left.Default, right.Default...),
	}
}

func annotationValues(value annotation) []string {
	result := appendUnique(nil, value.TTS...)
	return appendUnique(result, value.Default...)
}

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

func leftPadHex(value string) string {
	if len(value)%2 != 0 {
		return "0" + value
	}
	return value
}

func stripVariationSelectors(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\ufe0e' || r == '\ufe0f' {
			return -1
		}
		return r
	}, value)
}

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
