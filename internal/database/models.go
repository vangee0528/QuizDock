package database

import "github.com/quizdock/quizdock/internal/qbank"

type Bank struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Locale        string `json:"locale"`
	Exam          string `json:"exam"`
	Level         string `json:"level"`
	Subject       string `json:"subject"`
	License       string `json:"license"`
	Description   string `json:"description"`
	QuestionCount int    `json:"question_count"`
	Installed     bool   `json:"installed"`
	Enabled       bool   `json:"enabled"`
	ImportedAt    string `json:"imported_at"`
	UpdatedAt     string `json:"updated_at"`
}

type QuestionSummary struct {
	UID         string `json:"uid"`
	BankID      string `json:"bank_id"`
	BankName    string `json:"bank_name"`
	QuestionID  string `json:"question_id"`
	Title       string `json:"title"`
	Chapter     string `json:"chapter"`
	Topic       string `json:"topic"`
	Exam        string `json:"exam"`
	Type        string `json:"type"`
	LastCorrect *bool  `json:"last_correct"`
}

type QuestionDetail struct {
	QuestionSummary
	Stem        string       `json:"stem_md"`
	Explanation string       `json:"explanation_md,omitempty"`
	Tags        []string     `json:"tags"`
	Parts       []qbank.Part `json:"parts"`
	Starred     bool         `json:"starred"`
	AssetBase   string       `json:"asset_base"`
}

type QueueFilter struct {
	Mode    string
	BankIDs []string
	Chapter string
	Tag     string
	Exam    string
	Limit   int
	DueDate string
}

type Progress struct {
	ScopeKey     string         `json:"scope_key"`
	Mode         string         `json:"mode"`
	Filters      map[string]any `json:"filters"`
	CurrentUID   string         `json:"current_uid"`
	CurrentIndex int            `json:"current_index"`
	UpdatedAt    string         `json:"updated_at"`
}

type Settings struct {
	TrackWrong     bool   `json:"track_wrong"`
	DailyTarget    int    `json:"daily_target"`
	RequiredStreak int    `json:"required_streak"`
	AIURL          string `json:"ai_url"`
}

type Stats struct {
	Total      int     `json:"total"`
	Attempted  int     `json:"attempted"`
	Mastered   int     `json:"mastered"`
	Accuracy   float64 `json:"accuracy"`
	DueReviews int     `json:"due_reviews"`
}

type Meta struct {
	Version  string   `json:"version"`
	Banks    []Bank   `json:"banks"`
	Chapters []string `json:"chapters"`
	Tags     []string `json:"tags"`
	Exams    []string `json:"exams"`
	Stats    Stats    `json:"stats"`
	Settings Settings `json:"settings"`
}

type AnswerResult struct {
	Correct        bool                      `json:"correct"`
	CorrectAnswers map[string][]string       `json:"correct_answers"`
	CorrectOptions map[string][]qbank.Option `json:"correct_options"`
	Mastery        *Mastery                  `json:"mastery,omitempty"`
}

type Mastery struct {
	WrongCount         int    `json:"wrong_count"`
	CorrectCount       int    `json:"correct_count"`
	ConsecutiveCorrect int    `json:"consecutive_correct"`
	Status             string `json:"status"`
	DueDate            string `json:"due_date"`
}
