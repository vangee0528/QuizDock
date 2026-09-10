package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quizdock/quizdock/internal/qbank"
)

type ImportResult struct {
	BankID    string `json:"bank_id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Questions int    `json:"questions"`
	Assets    int    `json:"assets"`
	Updated   bool   `json:"updated"`
}

func (s *Store) ImportPackage(ctx context.Context, pkg *qbank.Package) (ImportResult, error) {
	manifestJSON, err := json.Marshal(pkg.Manifest)
	if err != nil {
		return ImportResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer tx.Rollback()
	var existingVersion string
	existingErr := tx.QueryRowContext(ctx, "SELECT version FROM question_banks WHERE id=?", pkg.Manifest.ID).Scan(&existingVersion)
	updated := existingErr == nil
	if existingErr != nil && existingErr != sql.ErrNoRows {
		return ImportResult{}, existingErr
	}
	timestamp := now()
	_, err = tx.ExecContext(ctx, `
        INSERT INTO question_banks(
            id,name,version,locale,exam,level,subject,license,description,manifest_json,
            archive_hash,question_count,installed,enabled,imported_at,updated_at
        ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,1,?,?)
		ON CONFLICT(id) DO UPDATE SET
            name=excluded.name,version=excluded.version,locale=excluded.locale,
            exam=excluded.exam,level=excluded.level,subject=excluded.subject,
            license=excluded.license,description=excluded.description,
            manifest_json=excluded.manifest_json,archive_hash=excluded.archive_hash,
			question_count=excluded.question_count,installed=1,enabled=1,updated_at=excluded.updated_at`,
		pkg.Manifest.ID, pkg.Manifest.Name, pkg.Manifest.Version, pkg.Manifest.Locale,
		pkg.Manifest.Exam, pkg.Manifest.Level, pkg.Manifest.Subject, pkg.Manifest.License,
		pkg.Manifest.Description, string(manifestJSON), pkg.ArchiveHash, len(pkg.Questions), timestamp, timestamp,
	)
	if err != nil {
		return ImportResult{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE questions SET active=0 WHERE bank_id=?", pkg.Manifest.ID); err != nil {
		return ImportResult{}, err
	}
	for _, question := range pkg.Questions {
		uid := pkg.Manifest.ID + ":" + question.ID
		ready := len(question.Parts) > 0
		for _, part := range question.Parts {
			hasCorrect := false
			for _, option := range part.Options {
				hasCorrect = hasCorrect || option.IsCorrect
			}
			ready = ready && len(part.Options) > 1 && hasCorrect
		}
		_, err := tx.ExecContext(ctx, `
            INSERT INTO questions(
                uid,bank_id,question_id,title,chapter,topic,question_type,order_index,
                exam,difficulty,stem_md,explanation_md,raw_markdown,ready,active,updated_at
            ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?)
            ON CONFLICT(uid) DO UPDATE SET
                title=excluded.title,chapter=excluded.chapter,topic=excluded.topic,
                question_type=excluded.question_type,order_index=excluded.order_index,
                exam=excluded.exam,difficulty=excluded.difficulty,stem_md=excluded.stem_md,
                explanation_md=excluded.explanation_md,raw_markdown=excluded.raw_markdown,
                ready=excluded.ready,active=1,updated_at=excluded.updated_at`,
			uid, pkg.Manifest.ID, question.ID, question.Title, question.Chapter, question.Topic,
			question.Type, question.Order, question.Exam, question.Difficulty, question.Stem,
			question.Explanation, question.RawMarkdown, boolInt(ready), timestamp,
		)
		if err != nil {
			return ImportResult{}, fmt.Errorf("import question %s: %w", question.ID, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM question_parts WHERE question_uid=?", uid); err != nil {
			return ImportResult{}, err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM question_tags WHERE question_uid=?", uid); err != nil {
			return ImportResult{}, err
		}
		for _, part := range question.Parts {
			if _, err := tx.ExecContext(ctx, "INSERT INTO question_parts(question_uid,part_index,label,prompt_md) VALUES(?,?,?,?)", uid, part.Index, part.Label, part.Prompt); err != nil {
				return ImportResult{}, err
			}
			for order, option := range part.Options {
				if _, err := tx.ExecContext(ctx, `INSERT INTO question_options(question_uid,part_index,option_order,label,body_md,is_correct) VALUES(?,?,?,?,?,?)`, uid, part.Index, order+1, option.Label, option.Body, boolInt(option.IsCorrect)); err != nil {
					return ImportResult{}, err
				}
			}
		}
		seenTags := make(map[string]bool)
		for _, tag := range append(question.Tags, question.Topic, question.Chapter) {
			tag = strings.TrimSpace(tag)
			if tag == "" || seenTags[tag] {
				continue
			}
			seenTags[tag] = true
			if _, err := tx.ExecContext(ctx, "INSERT INTO question_tags(question_uid,tag) VALUES(?,?)", uid, tag); err != nil {
				return ImportResult{}, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM assets WHERE bank_id=?", pkg.Manifest.ID); err != nil {
		return ImportResult{}, err
	}
	for _, asset := range pkg.Assets {
		if _, err := tx.ExecContext(ctx, "INSERT INTO assets(bank_id,path,mime_type,sha256,data) VALUES(?,?,?,?,?)", pkg.Manifest.ID, asset.Path, asset.MIME, asset.SHA256, asset.Data); err != nil {
			return ImportResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{BankID: pkg.Manifest.ID, Name: pkg.Manifest.Name, Version: pkg.Manifest.Version, Questions: len(pkg.Questions), Assets: len(pkg.Assets), Updated: updated}, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
