package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quizdock/quizdock/internal/qbank"
)

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRowContext(ctx, `SELECT track_wrong,daily_target,required_streak,ai_url,
		auto_submit,auto_next,arrow_keys FROM app_settings WHERE id=1`).
		Scan(&settings.TrackWrong, &settings.DailyTarget, &settings.RequiredStreak, &settings.AIURL,
			&settings.AutoSubmit, &settings.AutoNext, &settings.ArrowKeys)
	return settings, err
}

func (s *Store) SaveSettings(ctx context.Context, settings Settings) error {
	if settings.DailyTarget < 1 || settings.DailyTarget > 500 {
		return fmt.Errorf("daily_target must be between 1 and 500")
	}
	if settings.RequiredStreak < 1 || settings.RequiredStreak > 20 {
		return fmt.Errorf("required_streak must be between 1 and 20")
	}
	if !strings.HasPrefix(settings.AIURL, "https://") && !strings.HasPrefix(settings.AIURL, "http://") {
		return fmt.Errorf("ai_url must use http or https")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE app_settings SET track_wrong=?,daily_target=?,required_streak=?,ai_url=?,
		auto_submit=?,auto_next=?,arrow_keys=?,updated_at=? WHERE id=1`,
		boolInt(settings.TrackWrong), settings.DailyTarget, settings.RequiredStreak, settings.AIURL,
		boolInt(settings.AutoSubmit), boolInt(settings.AutoNext), boolInt(settings.ArrowKeys), now())
	return err
}

func (s *Store) SubmitAnswer(ctx context.Context, uid string, answers map[string][]string, durationMS int) (AnswerResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AnswerResult{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM questions WHERE uid=? AND active=1 AND ready=1", uid).Scan(&exists); err != nil {
		return AnswerResult{}, err
	}
	if exists == 0 {
		return AnswerResult{}, ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT part_index,label,body_md,is_correct FROM question_options
        WHERE question_uid=? ORDER BY part_index,option_order`, uid)
	if err != nil {
		return AnswerResult{}, err
	}
	correctLabels := make(map[string][]string)
	correctOptions := make(map[string][]qbank.Option)
	for rows.Next() {
		var part int
		var option qbank.Option
		if err := rows.Scan(&part, &option.Label, &option.Body, &option.IsCorrect); err != nil {
			rows.Close()
			return AnswerResult{}, err
		}
		if option.IsCorrect {
			key := fmt.Sprintf("%d", part)
			correctLabels[key] = append(correctLabels[key], option.Label)
			correctOptions[key] = append(correctOptions[key], option)
		}
	}
	rows.Close()
	correct := len(correctLabels) > 0
	for part, expected := range correctLabels {
		selected := append([]string(nil), answers[part]...)
		sort.Strings(selected)
		sort.Strings(expected)
		if strings.Join(selected, "\x00") != strings.Join(expected, "\x00") {
			correct = false
		}
	}
	if len(answers) != len(correctLabels) {
		correct = false
	}
	answersJSON, _ := json.Marshal(answers)
	if durationMS < 0 {
		durationMS = 0
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO attempts(question_uid,answers_json,correct,duration_ms,created_at) VALUES(?,?,?,?,?)", uid, string(answersJSON), boolInt(correct), durationMS, now()); err != nil {
		return AnswerResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO question_attempt_stats(question_uid,attempt_count,correct_count)
		VALUES(?,1,?) ON CONFLICT(question_uid) DO UPDATE SET
		attempt_count=attempt_count+1,correct_count=correct_count+excluded.correct_count`, uid, boolInt(correct)); err != nil {
		return AnswerResult{}, err
	}
	var trackWrong bool
	var requiredStreak int
	if err := tx.QueryRowContext(ctx, "SELECT track_wrong,required_streak FROM app_settings WHERE id=1").Scan(&trackWrong, &requiredStreak); err != nil {
		return AnswerResult{}, err
	}
	mastery, err := updateMastery(ctx, tx, uid, correct, trackWrong, requiredStreak)
	if err != nil {
		return AnswerResult{}, err
	}
	stats, err := loadStats(ctx, tx)
	if err != nil {
		return AnswerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AnswerResult{}, err
	}
	return AnswerResult{
		Correct: correct, CorrectAnswers: correctLabels, CorrectOptions: correctOptions,
		Mastery: mastery, Stats: stats,
	}, nil
}

func updateMastery(ctx context.Context, tx *sql.Tx, uid string, correct, trackWrong bool, requiredStreak int) (*Mastery, error) {
	var current Mastery
	err := tx.QueryRowContext(ctx, `SELECT wrong_count,correct_count,consecutive_correct,status,due_date
        FROM question_mastery WHERE question_uid=?`, uid).
		Scan(&current.WrongCount, &current.CorrectCount, &current.ConsecutiveCorrect, &current.Status, &current.DueDate)
	exists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if !correct && !trackWrong {
		return nil, nil
	}
	if correct && !exists {
		return nil, nil
	}
	nowTime := time.Now()
	if !correct {
		current.WrongCount++
		current.ConsecutiveCorrect = 0
		current.Status = "learning"
		current.DueDate = nowTime.Format("2006-01-02")
		_, err = tx.ExecContext(ctx, `INSERT INTO question_mastery(
            question_uid,wrong_count,correct_count,consecutive_correct,status,due_date,last_wrong_at,updated_at
          ) VALUES(?,?,?,?,?,?,?,?)
          ON CONFLICT(question_uid) DO UPDATE SET wrong_count=excluded.wrong_count,
            consecutive_correct=0,status='learning',due_date=excluded.due_date,
            last_wrong_at=excluded.last_wrong_at,updated_at=excluded.updated_at`,
			uid, current.WrongCount, current.CorrectCount, 0, current.Status, current.DueDate, now(), now())
	} else {
		current.CorrectCount++
		current.ConsecutiveCorrect++
		if current.ConsecutiveCorrect >= requiredStreak {
			current.Status = "mastered"
			current.DueDate = ""
		} else {
			current.Status = "reviewing"
			days := 1
			if current.ConsecutiveCorrect >= 2 {
				days = 3
			}
			current.DueDate = nowTime.AddDate(0, 0, days).Format("2006-01-02")
		}
		_, err = tx.ExecContext(ctx, `UPDATE question_mastery SET correct_count=?,consecutive_correct=?,
            status=?,due_date=?,last_correct_at=?,updated_at=? WHERE question_uid=?`,
			current.CorrectCount, current.ConsecutiveCorrect, current.Status, current.DueDate, now(), now(), uid)
	}
	if err != nil {
		return nil, err
	}
	return &current, nil
}
