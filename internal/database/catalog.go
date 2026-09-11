package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/quizdock/quizdock/internal/qbank"
)

func (s *Store) Banks(ctx context.Context, installedOnly bool) ([]Bank, error) {
	query := `SELECT id,name,version,locale,exam,level,subject,license,description,
                    question_count,installed,enabled,imported_at,updated_at
             FROM question_banks`
	if installedOnly {
		query += " WHERE installed=1"
	}
	query += " ORDER BY name,id"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Bank, 0)
	for rows.Next() {
		var bank Bank
		if err := rows.Scan(&bank.ID, &bank.Name, &bank.Version, &bank.Locale, &bank.Exam,
			&bank.Level, &bank.Subject, &bank.License, &bank.Description, &bank.QuestionCount,
			&bank.Installed, &bank.Enabled, &bank.ImportedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, bank)
	}
	return result, rows.Err()
}

func (s *Store) SetBankEnabled(ctx context.Context, bankID string, enabled bool) error {
	result, err := s.db.ExecContext(ctx, "UPDATE question_banks SET enabled=?,updated_at=? WHERE id=? AND installed=1", boolInt(enabled), now(), bankID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveBank(ctx context.Context, bankID string, purgeLearning bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM question_banks WHERE id=? AND installed=1", bankID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	if purgeLearning {
		for _, statement := range []string{
			"DELETE FROM attempts WHERE question_uid IN (SELECT uid FROM questions WHERE bank_id=?)",
			"DELETE FROM question_mastery WHERE question_uid IN (SELECT uid FROM questions WHERE bank_id=?)",
			"DELETE FROM question_state WHERE question_uid IN (SELECT uid FROM questions WHERE bank_id=?)",
			"DELETE FROM study_progress WHERE current_uid IN (SELECT uid FROM questions WHERE bank_id=?)",
			"DELETE FROM questions WHERE bank_id=?",
			"DELETE FROM question_banks WHERE id=?",
		} {
			if _, err := tx.ExecContext(ctx, statement, bankID); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, "UPDATE question_banks SET installed=0,enabled=0,updated_at=? WHERE id=?", now(), bankID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE questions SET active=0 WHERE bank_id=?", bankID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM assets WHERE bank_id=?", bankID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Meta(ctx context.Context, version string) (Meta, error) {
	banks, err := s.Banks(ctx, true)
	if err != nil {
		return Meta{}, err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return Meta{}, err
	}
	meta := Meta{
		Version: version, Banks: banks, Settings: settings,
		Chapters: []string{}, Tags: []string{}, Exams: []string{},
	}
	meta.Chapters, err = s.distinctQuestionValues(ctx, "chapter")
	if err != nil {
		return Meta{}, err
	}
	meta.Exams, err = s.distinctQuestionValues(ctx, "exam")
	if err != nil {
		return Meta{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT qt.tag
        FROM question_tags qt JOIN questions q ON q.uid=qt.question_uid
        JOIN question_banks b ON b.id=q.bank_id
        WHERE q.active=1 AND q.ready=1 AND b.installed=1 AND b.enabled=1
        ORDER BY qt.tag`)
	if err != nil {
		return Meta{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return Meta{}, err
		}
		meta.Tags = append(meta.Tags, value)
	}
	rows.Close()
	meta.Stats, err = loadStats(ctx, s.db)
	if err != nil {
		return Meta{}, err
	}
	return meta, nil
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadStats(ctx context.Context, queryer rowQuerier) (Stats, error) {
	var stats Stats
	var correct, attempts int
	err := queryer.QueryRowContext(ctx, `SELECT
		COUNT(CASE WHEN a.attempt_count>0 THEN 1 END),
		COALESCE(SUM(a.attempt_count),0), COALESCE(SUM(a.correct_count),0),
		COUNT(CASE WHEN m.status='mastered' THEN 1 END),
		COUNT(CASE WHEN m.status!='mastered' AND m.due_date<=? THEN 1 END),
		COUNT(*)
      FROM questions q JOIN question_banks b ON b.id=q.bank_id
	  LEFT JOIN question_attempt_stats a ON a.question_uid=q.uid
      LEFT JOIN question_mastery m ON m.question_uid=q.uid
      WHERE q.active=1 AND q.ready=1 AND b.installed=1 AND b.enabled=1`, today()).
		Scan(&stats.Attempted, &attempts, &correct, &stats.Mastered, &stats.DueReviews, &stats.Total)
	if err != nil {
		return Stats{}, err
	}
	if attempts > 0 {
		stats.Accuracy = float64(correct) * 100 / float64(attempts)
	}
	return stats, nil
}

func (s *Store) distinctQuestionValues(ctx context.Context, column string) ([]string, error) {
	if column != "chapter" && column != "exam" {
		return nil, fmt.Errorf("unsupported distinct column")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT DISTINCT q.%s FROM questions q
        JOIN question_banks b ON b.id=q.bank_id
        WHERE q.active=1 AND q.ready=1 AND q.%s!='' AND b.installed=1 AND b.enabled=1
        ORDER BY q.%s`, column, column, column))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) QuestionQueue(ctx context.Context, filter QueueFilter) ([]QueueItem, error) {
	clauses := []string{"q.active=1", "q.ready=1", "b.installed=1", "b.enabled=1"}
	args := make([]any, 0)
	if len(filter.BankIDs) > 0 {
		clauses = append(clauses, "q.bank_id IN ("+placeholders(len(filter.BankIDs))+")")
		args = append(args, stringsToAny(filter.BankIDs)...)
	}
	if filter.Chapter != "" {
		clauses = append(clauses, "q.chapter=?")
		args = append(args, filter.Chapter)
	}
	if filter.Tag != "" {
		clauses = append(clauses, "EXISTS(SELECT 1 FROM question_tags qt WHERE qt.question_uid=q.uid AND qt.tag=?)")
		args = append(args, filter.Tag)
	}
	if filter.Exam != "" {
		clauses = append(clauses, "q.exam=?")
		args = append(args, filter.Exam)
	}
	if filter.Mode == "review" {
		clauses = append(clauses, "EXISTS(SELECT 1 FROM question_mastery m WHERE m.question_uid=q.uid AND m.status!='mastered' AND m.due_date<=?)")
		args = append(args, filter.DueDate)
	}
	order := "b.name,q.order_index,q.question_id"
	if filter.Mode == "random" {
		order = "RANDOM()"
	}
	limit := filter.Limit
	if limit <= 0 || limit > 10_000 {
		limit = 10_000
	}
	args = append(args, limit)
	query := `SELECT q.uid,q.title,
        (SELECT a.correct FROM attempts a WHERE a.question_uid=q.uid ORDER BY a.id DESC LIMIT 1)
        FROM questions q JOIN question_banks b ON b.id=q.bank_id
        WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY ` + order + ` LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]QueueItem, 0)
	for rows.Next() {
		var question QueueItem
		var last sql.NullBool
		if err := rows.Scan(&question.UID, &question.Title, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			value := last.Bool
			question.LastCorrect = &value
		}
		result = append(result, question)
	}
	return result, rows.Err()
}

func (s *Store) Question(ctx context.Context, uid string) (QuestionDetail, error) {
	questions, err := s.Questions(ctx, []string{uid})
	if err != nil {
		return QuestionDetail{}, err
	}
	if len(questions) == 0 {
		return QuestionDetail{}, ErrNotFound
	}
	return questions[0], nil
}

func (s *Store) Questions(ctx context.Context, uids []string) ([]QuestionDetail, error) {
	uniqueUIDs := make([]string, 0, len(uids))
	seen := make(map[string]struct{}, len(uids))
	for _, uid := range uids {
		if uid == "" {
			continue
		}
		if _, exists := seen[uid]; exists {
			continue
		}
		seen[uid] = struct{}{}
		uniqueUIDs = append(uniqueUIDs, uid)
	}
	if len(uniqueUIDs) == 0 {
		return []QuestionDetail{}, nil
	}

	args := stringsToAny(uniqueUIDs)
	rows, err := s.db.QueryContext(ctx, `SELECT q.uid,q.bank_id,b.name,q.question_id,q.title,
        q.chapter,q.topic,q.exam,q.question_type,q.stem_md,q.explanation_md,
        COALESCE(qs.starred,0),
        (SELECT a.correct FROM attempts a WHERE a.question_uid=q.uid ORDER BY a.id DESC LIMIT 1),
		q.active,b.installed
      FROM questions q JOIN question_banks b ON b.id=q.bank_id
      LEFT JOIN question_state qs ON qs.question_uid=q.uid
		WHERE q.uid IN (`+placeholders(len(uniqueUIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byUID := make(map[string]*QuestionDetail, len(uniqueUIDs))
	for rows.Next() {
		var question QuestionDetail
		var last sql.NullBool
		var active, installed bool
		if err := rows.Scan(&question.UID, &question.BankID, &question.BankName, &question.QuestionID,
			&question.Title, &question.Chapter, &question.Topic, &question.Exam, &question.Type,
			&question.Stem, &question.Explanation, &question.Starred, &last, &active, &installed); err != nil {
			return nil, err
		}
		if !active || !installed {
			continue
		}
		if last.Valid {
			value := last.Bool
			question.LastCorrect = &value
		}
		question.AssetBase = "/api/v1/assets/" + url.PathEscape(question.BankID) + "/"
		question.Tags = []string{}
		question.Parts = []qbank.Part{}
		byUID[question.UID] = &question
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(byUID) == 0 {
		return []QuestionDetail{}, nil
	}

	tagRows, err := s.db.QueryContext(ctx, `SELECT question_uid,tag FROM question_tags
		WHERE question_uid IN (`+placeholders(len(uniqueUIDs))+`) ORDER BY question_uid,tag`, args...)
	if err != nil {
		return nil, err
	}
	for tagRows.Next() {
		var uid, tag string
		if err := tagRows.Scan(&uid, &tag); err != nil {
			tagRows.Close()
			return nil, err
		}
		if question := byUID[uid]; question != nil {
			question.Tags = append(question.Tags, tag)
		}
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		return nil, err
	}
	tagRows.Close()

	partRows, err := s.db.QueryContext(ctx, `SELECT p.question_uid,p.part_index,p.label,p.prompt_md,o.label,o.body_md
		FROM question_parts p LEFT JOIN question_options o
		ON o.question_uid=p.question_uid AND o.part_index=p.part_index
		WHERE p.question_uid IN (`+placeholders(len(uniqueUIDs))+`)
		ORDER BY p.question_uid,p.part_index,o.option_order`, args...)
	if err != nil {
		return nil, err
	}
	partIndexes := make(map[string]map[int]int, len(byUID))
	for partRows.Next() {
		var uid, label, prompt string
		var partIndex int
		var optionLabel, optionBody sql.NullString
		if err := partRows.Scan(&uid, &partIndex, &label, &prompt, &optionLabel, &optionBody); err != nil {
			partRows.Close()
			return nil, err
		}
		question := byUID[uid]
		if question == nil {
			continue
		}
		indexes := partIndexes[uid]
		if indexes == nil {
			indexes = make(map[int]int)
			partIndexes[uid] = indexes
		}
		position, exists := indexes[partIndex]
		if !exists {
			position = len(question.Parts)
			indexes[partIndex] = position
			question.Parts = append(question.Parts, qbank.Part{
				Index: partIndex, Label: label, Prompt: prompt, Options: []qbank.Option{},
			})
		}
		if optionLabel.Valid {
			question.Parts[position].Options = append(question.Parts[position].Options, qbank.Option{
				Label: optionLabel.String, Body: optionBody.String,
			})
		}
	}
	if err := partRows.Err(); err != nil {
		partRows.Close()
		return nil, err
	}
	partRows.Close()

	result := make([]QuestionDetail, 0, len(byUID))
	for _, uid := range uniqueUIDs {
		if question := byUID[uid]; question != nil {
			result = append(result, *question)
		}
	}
	return result, nil
}

func (s *Store) Asset(ctx context.Context, bankID, assetPath string) ([]byte, string, string, error) {
	assetPath = strings.TrimPrefix(assetPath, "/")
	if !strings.HasPrefix(assetPath, "assets/") {
		assetPath = "assets/" + assetPath
	}
	var data []byte
	var mimeType, hash string
	err := s.db.QueryRowContext(ctx, "SELECT data,mime_type,sha256 FROM assets WHERE bank_id=? AND path=?", bankID, assetPath).Scan(&data, &mimeType, &hash)
	if err == sql.ErrNoRows {
		return nil, "", "", ErrNotFound
	}
	return data, mimeType, hash, err
}

func (s *Store) SetStarred(ctx context.Context, uid string, starred bool) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO question_state(question_uid,starred,updated_at) VALUES(?,?,?)
        ON CONFLICT(question_uid) DO UPDATE SET starred=excluded.starred,updated_at=excluded.updated_at`, uid, boolInt(starred), now())
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Progress(ctx context.Context, scopeKey string) (*Progress, error) {
	var progress Progress
	var filtersJSON string
	err := s.db.QueryRowContext(ctx, "SELECT scope_key,mode,filters_json,current_uid,current_index,updated_at FROM study_progress WHERE scope_key=?", scopeKey).
		Scan(&progress.ScopeKey, &progress.Mode, &filtersJSON, &progress.CurrentUID, &progress.CurrentIndex, &progress.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(filtersJSON), &progress.Filters); err != nil {
		return nil, err
	}
	return &progress, nil
}

func (s *Store) SaveProgress(ctx context.Context, progress Progress) error {
	filters, err := json.Marshal(progress.Filters)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO study_progress(scope_key,mode,filters_json,current_uid,current_index,updated_at)
        VALUES(?,?,?,?,?,?) ON CONFLICT(scope_key) DO UPDATE SET mode=excluded.mode,
        filters_json=excluded.filters_json,current_uid=excluded.current_uid,
        current_index=excluded.current_index,updated_at=excluded.updated_at`,
		progress.ScopeKey, progress.Mode, string(filters), progress.CurrentUID, progress.CurrentIndex, now())
	return err
}

func (s *Store) InstalledBankIDs(ctx context.Context) ([]string, error) {
	banks, err := s.Banks(ctx, true)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(banks))
	for _, bank := range banks {
		if bank.Enabled {
			ids = append(ids, bank.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
