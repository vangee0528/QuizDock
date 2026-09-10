package qbank

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testQuestion = `---
id: "q-000001"
title: "示例题"
chapter: "示例章节"
type: "single-choice"
order: 1
tags: ["示例"]
---

# 示例题

## 题目

下列哪一项正确？

## 选项

- **A.** 正确
- **B.** 错误

## 参考答案

- **A.** 正确
`

func TestPackAndLoad(t *testing.T) {
	root := t.TempDir()
	questions := filepath.Join(root, "questions")
	if err := os.Mkdir(questions, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"id":"example.bank","name":"示例题库","version":"1.0.0","question_count":1}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(questions, "q-000001.md"), []byte(testQuestion), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "example.qbank")
	summary, err := Pack(root, destination)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Questions != 1 {
		t.Fatalf("questions = %d, want 1", summary.Questions)
	}
	pkg, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got := pkg.Questions[0].Parts[0].Options[0]; got.Label != "A" || !got.IsCorrect {
		t.Fatalf("unexpected correct option: %#v", got)
	}
}

func TestRejectsPathTraversal(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Read(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("error = %v, want unsafe archive path", err)
	}
}

func TestMarkdownTableIsPreserved(t *testing.T) {
	raw := strings.Replace(testQuestion, "下列哪一项正确？", "| 字段 | 值 |\n| --- | --- |\n| A | 1 |", 1)
	question, err := ParseMarkdown("question.md", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(question.Stem, "| --- | --- |") {
		t.Fatalf("table was not preserved: %q", question.Stem)
	}
}
