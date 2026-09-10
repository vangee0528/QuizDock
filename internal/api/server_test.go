package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/quizdock/quizdock/internal/database"
	"github.com/quizdock/quizdock/internal/qbank"
)

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
