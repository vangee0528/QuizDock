package qbank

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	headingPattern = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	h1Pattern      = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	optionPattern  = regexp.MustCompile(`(?m)^\s*-\s+(?:\*\*)?([A-Za-z0-9]+)[.:、．]?\*\*\s*(.*)$`)
	answerPattern  = regexp.MustCompile(`(?i)(?:\*\*)?([A-Z])(?:[.、．:]|\*\*)`)
	partPattern    = regexp.MustCompile(`(?m)^###\s+(.+?)\s*$`)
)

func ParseMarkdown(path string, raw []byte) (Question, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	metadata, body, err := parseFrontMatter(text)
	if err != nil {
		return Question{}, fmt.Errorf("%s: %w", path, err)
	}
	sections := splitSections(body)
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		if match := h1Pattern.FindStringSubmatch(body); len(match) > 1 {
			title = strings.TrimSpace(match[1])
		}
	}
	if metadata.ID == "" {
		return Question{}, fmt.Errorf("%s: front matter is missing id", path)
	}
	if title == "" {
		title = metadata.ID
	}
	questionType := normalizeType(metadata.Type)
	answer := firstSection(sections, "参考答案", "Answer")
	var stem string
	var parts []Part
	if optionText := firstSection(sections, "各空选项", "Parts"); optionText != "" {
		stem = firstSection(sections, "共用题干", "题目", "Question")
		parts, err = parseCompoundParts(optionText, answer)
		if err != nil {
			return Question{}, fmt.Errorf("%s: %w", path, err)
		}
		questionType = "compound-choice"
	} else if optionText := firstSection(sections, "选项", "Options"); optionText != "" {
		stem = firstSection(sections, "题目", "Question", "共用题干")
		options := parseOptions(optionText, answerKeys(answer))
		if len(options) == 0 {
			return Question{}, fmt.Errorf("%s: no options were parsed", path)
		}
		parts = []Part{{Index: 1, Label: "本题", Options: options}}
		if questionType == "content" || questionType == "" {
			questionType = "single-choice"
		}
	} else {
		stem = firstSection(sections, "题目", "Question", "案例说明", "问题", "共用题干")
		questionType = "content"
	}
	if strings.TrimSpace(stem) == "" {
		return Question{}, fmt.Errorf("%s: no question stem", path)
	}
	order := metadata.Order
	if order == 0 {
		order = metadata.Sequence
	}
	if order == 0 {
		prefix := strings.SplitN(filepath.Base(path), "-", 2)[0]
		order, _ = strconv.Atoi(prefix)
	}
	topic := strings.TrimSpace(metadata.Topic)
	if topic == "" {
		topic = strings.TrimSpace(metadata.Chapter)
	}
	return Question{
		ID: metadata.ID, Title: title, Chapter: strings.TrimSpace(metadata.Chapter),
		Topic: topic, Type: questionType, Order: order, Tags: uniqueStrings(metadata.Tags),
		Exam: strings.TrimSpace(metadata.Exam), Difficulty: strings.TrimSpace(metadata.Difficulty),
		Stem: strings.TrimSpace(stem), Explanation: strings.TrimSpace(firstSection(sections, "解析", "Explanation")),
		Parts: parts, RawMarkdown: text, Path: path,
	}, nil
}

func parseFrontMatter(text string) (Metadata, string, error) {
	var metadata Metadata
	if !strings.HasPrefix(text, "---\n") {
		return metadata, text, fmt.Errorf("missing YAML front matter")
	}
	remaining := text[4:]
	end := strings.Index(remaining, "\n---\n")
	if end < 0 {
		return metadata, text, fmt.Errorf("unterminated YAML front matter")
	}
	if err := yaml.Unmarshal([]byte(remaining[:end]), &metadata); err != nil {
		return metadata, text, fmt.Errorf("invalid YAML front matter: %w", err)
	}
	return metadata, remaining[end+5:], nil
}

func splitSections(body string) map[string]string {
	result := make(map[string]string)
	matches := headingPattern.FindAllStringSubmatchIndex(body, -1)
	for i, match := range matches {
		name := strings.TrimSpace(body[match[2]:match[3]])
		start := match[1]
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		result[name] = strings.TrimSpace(body[start:end])
	}
	return result
}

func firstSection(sections map[string]string, names ...string) string {
	for _, name := range names {
		if value := sections[name]; value != "" {
			return value
		}
	}
	return ""
}

func normalizeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "选择题", "单选题", "single", "single-choice":
		return "single-choice"
	case "组合选择题", "compound", "compound-choice":
		return "compound-choice"
	case "案例题", "简答题", "content":
		return "content"
	default:
		return ""
	}
}

func parseOptions(text string, answers map[string]bool) []Option {
	matches := optionPattern.FindAllStringSubmatchIndex(text, -1)
	options := make([]Option, 0, len(matches))
	for i, match := range matches {
		label := strings.ToUpper(strings.TrimSpace(text[match[2]:match[3]]))
		bodyStart := match[4]
		bodyEnd := match[5]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		body := strings.TrimSpace(text[bodyStart:bodyEnd] + text[match[1]:end])
		options = append(options, Option{Label: label, Body: body, IsCorrect: answers[label]})
	}
	return options
}

func answerKeys(text string) map[string]bool {
	result := make(map[string]bool)
	for _, match := range answerPattern.FindAllStringSubmatch(text, -1) {
		result[strings.ToUpper(match[1])] = true
	}
	if len(result) == 0 {
		for _, token := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '、' || r == ' ' || r == '\n' }) {
			token = strings.ToUpper(strings.TrimSpace(token))
			if len(token) == 1 && token[0] >= 'A' && token[0] <= 'Z' {
				result[token] = true
			}
		}
	}
	return result
}

func parseCompoundParts(text, answer string) ([]Part, error) {
	matches := partPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("compound question has no part headings")
	}
	answerSections := splitPartAnswers(answer)
	parts := make([]Part, 0, len(matches))
	for i, match := range matches {
		label := strings.TrimSpace(text[match[2]:match[3]])
		start := match[1]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		keys := answerSections[i+1]
		options := parseOptions(strings.TrimSpace(text[start:end]), keys)
		if len(options) == 0 {
			return nil, fmt.Errorf("part %d has no options", i+1)
		}
		parts = append(parts, Part{Index: i + 1, Label: label, Options: options})
	}
	return parts, nil
}

func splitPartAnswers(text string) map[int]map[string]bool {
	result := make(map[int]map[string]bool)
	linePattern := regexp.MustCompile(`(?m)^\s*-?\s*(?:第\s*)?(\d+)\s*(?:空|题)?[^A-Z\n]*([A-Z])`)
	for _, match := range linePattern.FindAllStringSubmatch(text, -1) {
		index, _ := strconv.Atoi(match[1])
		if result[index] == nil {
			result[index] = make(map[string]bool)
		}
		result[index][strings.ToUpper(match[2])] = true
	}
	if len(result) == 0 {
		keys := make([]string, 0)
		for key := range answerKeys(text) {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			result[index+1] = map[string]bool{key: true}
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
