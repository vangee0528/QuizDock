package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/quizdock/quizdock/internal/database"
	"github.com/quizdock/quizdock/internal/qbank"
	"github.com/quizdock/quizdock/internal/releases"
)

func apiSamplePackage() *qbank.Package {
	return &qbank.Package{
		Manifest: qbank.Manifest{SchemaVersion: 1, ID: "example.bank", Name: "Example", Version: "1.0.0", QuestionCount: 1},
		Questions: []qbank.Question{{
			ID: "q-1", Title: "Example-q-1", Type: "single-choice", Stem: "Question", RawMarkdown: "example",
			Parts: []qbank.Part{{Index: 1, Label: "Question", Options: []qbank.Option{
				{Label: "A", Body: "Correct", IsCorrect: true},
				{Label: "B", Body: "Wrong"},
			}}},
		}},
		ArchiveHash: "test",
	}
}

func TestStartPracticeReturnsQueueProgressAndQuestion(t *testing.T) {
	dataDir := t.TempDir()
	store, err := database.Open(filepath.Join(dataDir, "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ImportPackage(context.Background(), apiSamplePackage()); err != nil {
		t.Fatal(err)
	}
	scope := `sequence:{"banks":["example.bank"]}`
	if err := store.SaveProgress(context.Background(), database.Progress{
		ScopeKey: scope, Mode: "sequence", Filters: map[string]any{"banks": []string{"example.bank"}},
		CurrentUID: "example.bank:q-1", CurrentIndex: 0,
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(store, "test", dataDir, ""))
	defer server.Close()

	endpoint := server.URL + "/api/v1/practice/start?mode=sequence&limit=10000&bank=example.bank&scope=" + url.QueryEscape(scope)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var payload struct {
		Questions    []database.QueueItem     `json:"questions"`
		CurrentIndex int                      `json:"current_index"`
		Question     *database.QuestionDetail `json:"question"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Questions) != 1 || payload.CurrentIndex != 0 || payload.Question == nil {
		t.Fatalf("unexpected practice payload: %+v", payload)
	}
	if payload.Questions[0].UID != "example.bank:q-1" || payload.Question.UID != "example.bank:q-1" {
		t.Fatalf("unexpected current question: %+v", payload)
	}
}

func TestEncodedQuestionUIDRoute(t *testing.T) {
	dataDir := t.TempDir()
	store, err := database.Open(filepath.Join(dataDir, "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pkg := &qbank.Package{
		Manifest: qbank.Manifest{SchemaVersion: 1, ID: "example.bank", Name: "Example", Version: "1.0.0", QuestionCount: 1},
		Questions: []qbank.Question{{
			ID: "q-1", Title: "Example-q-1", Type: "single-choice", Stem: "Question", RawMarkdown: "example",
			Parts: []qbank.Part{{Index: 1, Label: "Question", Options: []qbank.Option{
				{Label: "A", Body: "Correct", IsCorrect: true},
				{Label: "B", Body: "Wrong"},
			}}},
		}},
		ArchiveHash: "test",
	}
	if _, err := store.ImportPackage(context.Background(), pkg); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(store, "test", dataDir, ""))
	defer server.Close()
	response, err := http.Get(server.URL + "/api/v1/questions/example.bank%3Aq-1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestInstallOfficialBankFromRelease(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "questions"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"id":"cn.ruankao.software-designer","name":"软件设计师题库","version":"0.1.1","question_count":1}`
	question := `---
id: "q-000001"
title: "计算机基础-q-000001"
chapter: "计算机基础"
type: "single-choice"
order: 1
tags: ["示例"]
---

# 计算机基础-q-000001

## 题目

哪一项正确？

## 选项

- **A.** 正确
- **B.** 错误

## 参考答案

- **A.** 正确
`
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "questions", "q-000001.md"), []byte(question), 0o644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "software-designer-0.1.1.qbank")
	if _, err := qbank.Pack(source, archivePath); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	var releaseServer *httptest.Server
	releaseServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `[{"tag_name":"qbank/software-designer/v0.1.1","html_url":"%s/release","draft":false,"prerelease":false,"assets":[{"name":"software-designer-0.1.1.qbank","browser_download_url":"%s/bank","size":%d}]}]`, releaseServer.URL, releaseServer.URL, len(archive))
		case "/bank":
			writer.Header().Set("Content-Type", "application/octet-stream")
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer releaseServer.Close()

	dataDir := t.TempDir()
	store, err := database.Open(filepath.Join(dataDir, "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := httptest.NewServer(NewWithOptions(store, Options{
		Version: "0.2.0", DataDir: dataDir,
		ReleaseClient: releases.NewClient(releaseServer.URL+"/releases", releaseServer.URL+"/release", releaseServer.Client()),
	}))
	defer server.Close()

	response, err := http.Post(server.URL+"/api/v1/official-banks/software-designer/install", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("install status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	banks, err := store.Banks(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(banks) != 1 || banks[0].Version != "0.1.1" {
		t.Fatalf("unexpected installed banks: %+v", banks)
	}
}

func TestProtectedAPIRequiresConfiguredLogin(t *testing.T) {
	dataDir := t.TempDir()
	store, err := database.Open(filepath.Join(dataDir, "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := httptest.NewServer(NewWithOptions(store, Options{
		Version: "test", DataDir: dataDir, AuthUsername: "tester", AuthPassword: "secret",
	}))
	defer server.Close()

	client := &http.Client{}
	response, err := client.Get(server.URL + "/api/v1/meta")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}

	payload, _ := json.Marshal(map[string]string{"username": "tester", "password": "secret"})
	response, err = client.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(response.Cookies()) != 1 {
		t.Fatalf("login status = %d, cookies = %d", response.StatusCode, len(response.Cookies()))
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/meta", nil)
	request.AddCookie(response.Cookies()[0])
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}
