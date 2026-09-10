package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/quizdock/quizdock/internal/qbank"
)

func samplePackage() *qbank.Package {
	return &qbank.Package{
		Manifest: qbank.Manifest{SchemaVersion: 1, ID: "example.bank", Name: "示例题库", Version: "1.0.0", QuestionCount: 1},
		Questions: []qbank.Question{{
			ID: "q-1", Title: "示例题", Chapter: "示例章节", Topic: "示例主题",
			Type: "single-choice", Order: 1, Stem: "正确答案是哪一个？", RawMarkdown: "example",
			Parts: []qbank.Part{{Index: 1, Label: "本题", Options: []qbank.Option{
				{Label: "A", Body: "正确", IsCorrect: true},
				{Label: "B", Body: "错误", IsCorrect: false},
			}}},
		}},
		ArchiveHash: "example-hash",
	}
}

func TestImportAndPracticePreserveLearningState(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ImportPackage(ctx, samplePackage()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.QuestionQueue(ctx, QueueFilter{Mode: "sequence", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 {
		t.Fatalf("queue length = %d, want 1", len(queue))
	}
	uid := "example.bank:q-1"
	result, err := store.SubmitAnswer(ctx, uid, map[string][]string{"1": {"B"}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Correct || result.Mastery == nil || result.Mastery.WrongCount != 1 {
		t.Fatalf("unexpected answer result: %#v", result)
	}
	updated := samplePackage()
	updated.Manifest.Version = "1.1.0"
	updated.Questions[0].Title = "更新后的示例题"
	if _, err := store.ImportPackage(ctx, updated); err != nil {
		t.Fatal(err)
	}
	meta, err := store.Meta(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Stats.Attempted != 1 || meta.Stats.DueReviews != 1 {
		t.Fatalf("learning state was not preserved: %#v", meta.Stats)
	}
}
