package qbank

import "time"

const SchemaVersion = 1

type Manifest struct {
	SchemaVersion     int    `json:"schema_version"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	Locale            string `json:"locale,omitempty"`
	Exam              string `json:"exam,omitempty"`
	Level             string `json:"level,omitempty"`
	Subject           string `json:"subject,omitempty"`
	QuestionCount     int    `json:"question_count"`
	MinimumAppVersion string `json:"minimum_app_version,omitempty"`
	License           string `json:"license,omitempty"`
	Description       string `json:"description,omitempty"`
}

type Metadata struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Chapter    string   `yaml:"chapter"`
	Topic      string   `yaml:"topic"`
	Type       string   `yaml:"type"`
	Order      int      `yaml:"order"`
	Sequence   int      `yaml:"sequence"`
	Tags       []string `yaml:"tags"`
	Exam       string   `yaml:"exam"`
	Difficulty string   `yaml:"difficulty"`
}

type Option struct {
	Label     string `json:"label"`
	Body      string `json:"body_md"`
	IsCorrect bool   `json:"is_correct,omitempty"`
}

type Part struct {
	Index   int      `json:"part_index"`
	Label   string   `json:"label"`
	Prompt  string   `json:"prompt_md,omitempty"`
	Options []Option `json:"options"`
}

type Question struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Chapter     string   `json:"chapter,omitempty"`
	Topic       string   `json:"topic,omitempty"`
	Type        string   `json:"type"`
	Order       int      `json:"order"`
	Tags        []string `json:"tags"`
	Exam        string   `json:"exam,omitempty"`
	Difficulty  string   `json:"difficulty,omitempty"`
	Stem        string   `json:"stem_md"`
	Explanation string   `json:"explanation_md,omitempty"`
	Parts       []Part   `json:"parts"`
	RawMarkdown string   `json:"-"`
	Path        string   `json:"-"`
}

type Asset struct {
	Path   string
	MIME   string
	SHA256 string
	Data   []byte
}

type Package struct {
	Manifest    Manifest
	Questions   []Question
	Assets      []Asset
	ArchiveHash string
	LoadedAt    time.Time
}

type Summary struct {
	Manifest    Manifest `json:"manifest"`
	Questions   int      `json:"questions"`
	Assets      int      `json:"assets"`
	ArchiveHash string   `json:"archive_sha256"`
}
