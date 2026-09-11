package database

import (
	"context"
	"errors"
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
	if result.Stats.Attempted != 1 || result.Stats.DueReviews != 1 || result.Stats.Total != 1 {
		t.Fatalf("answer result did not include updated stats: %#v", result.Stats)
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

func TestEmptyMetaUsesEmptyCollections(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meta, err := store.Meta(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Banks == nil || meta.Chapters == nil || meta.Tags == nil || meta.Exams == nil {
		t.Fatalf("empty metadata collections must not be nil: %#v", meta)
	}
}

func TestAttemptStatsMigrationBackfillsHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "quizdock.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportPackage(ctx, samplePackage()); err != nil {
		t.Fatal(err)
	}
	uid := "example.bank:q-1"
	if _, err := store.SubmitAnswer(ctx, uid, map[string][]string{"1": {"B"}}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitAnswer(ctx, uid, map[string][]string{"1": {"A"}}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "DROP TABLE question_attempt_stats"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=3"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	meta, err := reopened.Meta(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Stats.Attempted != 1 || meta.Stats.Total != 1 || meta.Stats.Accuracy != 50 {
		t.Fatalf("unexpected backfilled stats: %#v", meta.Stats)
	}
}

func TestQuestionsLoadsMultipleDetailsInRequestedOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "quizdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pkg := samplePackage()
	second := pkg.Questions[0]
	second.ID = "q-2"
	second.Title = "第二题"
	second.Parts = []qbank.Part{
		{Index: 1, Label: "第一空", Options: []qbank.Option{{Label: "A", Body: "甲", IsCorrect: true}, {Label: "B", Body: "乙"}}},
		{Index: 2, Label: "第二空", Options: []qbank.Option{{Label: "A", Body: "丙"}, {Label: "B", Body: "丁", IsCorrect: true}}},
	}
	pkg.Questions = append(pkg.Questions, second)
	pkg.Manifest.QuestionCount = len(pkg.Questions)
	if _, err := store.ImportPackage(context.Background(), pkg); err != nil {
		t.Fatal(err)
	}

	questions, err := store.Questions(context.Background(), []string{
		"example.bank:q-2", "example.bank:q-1", "example.bank:q-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 2 || questions[0].UID != "example.bank:q-2" || questions[1].UID != "example.bank:q-1" {
		t.Fatalf("unexpected question order: %+v", questions)
	}
	if len(questions[0].Parts) != 2 || len(questions[0].Parts[0].Options) != 2 || questions[0].Parts[1].Options[1].Body != "丁" {
		t.Fatalf("unexpected question parts: %+v", questions[0].Parts)
	}
	if err := store.RemoveBank(context.Background(), "example.bank", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Question(context.Background(), "example.bank:q-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("uninstalled question error = %v, want ErrNotFound", err)
	}
}

func TestSettingsPersistPracticePreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quizdock.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.AutoSubmit || settings.AutoNext || !settings.ArrowKeys {
		t.Fatalf("unexpected practice preference defaults: %#v", settings)
	}
	settings.AutoSubmit = true
	settings.AutoNext = true
	settings.ArrowKeys = false
	if err := store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.AutoSubmit || !persisted.AutoNext || persisted.ArrowKeys {
		t.Fatalf("practice preferences were not persisted: %#v", persisted)
	}
}
