package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SummaryTemplate represents a saved summarization prompt/template
type SummaryTemplate struct {
	ID                 string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name               string    `json:"name" gorm:"type:varchar(255);not null"`
	Description        *string   `json:"description,omitempty" gorm:"type:text"`
	Model              string    `json:"model" gorm:"type:varchar(255);not null;default:''"`
	Prompt             string    `json:"prompt" gorm:"type:text;not null"`
	IncludeSpeakerInfo bool      `json:"include_speaker_info" gorm:"default:false"`
	CreatedAt          time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (st *SummaryTemplate) BeforeCreate(tx *gorm.DB) error {
	if st.ID == "" {
		st.ID = uuid.New().String()
	}
	return nil
}

// SummarySetting stores global settings for summarization (single row)
type SummarySetting struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	DefaultModel string    `json:"default_model" gorm:"type:varchar(255);not null;default:''"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// Summary stores a generated summary linked to a transcription
type Summary struct {
	ID              string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TranscriptionID string    `json:"transcription_id" gorm:"type:varchar(36);index;not null"`
	TemplateID      *string   `json:"template_id,omitempty" gorm:"type:varchar(36)"`
	Model           string    `json:"model" gorm:"type:varchar(255);not null"`
	Content         string    `json:"content" gorm:"type:text;not null"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`

	// Local Minutes additions (additive columns). Rows written before these
	// columns existed, or by an older binary, take the defaults and are
	// reported as legacy drafts whose completion was never recorded.
	GenerationStatus string  `json:"generation_status" gorm:"type:varchar(32);not null;default:'legacy_unrecorded'"`
	Format           string  `json:"format" gorm:"type:varchar(32);not null;default:'legacy_markdown'"`
	Provider         string  `json:"provider" gorm:"type:varchar(50);not null;default:''"`
	AttemptID        *string `json:"attempt_id,omitempty" gorm:"type:varchar(36)"`
	TranscriptSHA256 string  `json:"transcript_sha256,omitempty" gorm:"type:varchar(64);not null;default:''"`
	// DraftJSON holds the checked evidence-linked draft (summary.StoredDraft).
	DraftJSON *string `json:"-" gorm:"type:text"`

	// Relationships
	Transcription TranscriptionJob `json:"transcription,omitempty" gorm:"foreignKey:TranscriptionID;constraint:OnDelete:CASCADE"`
}

// SummaryAttempt records one summary generation request and how it ended.
// Only a completed attempt creates a Summary; failed, cancelled, rejected or
// incomplete attempts are recorded here and never replace a saved summary.
type SummaryAttempt struct {
	ID                  string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TranscriptionID     string     `json:"transcription_id" gorm:"type:varchar(36);index;not null"`
	TemplateID          *string    `json:"template_id,omitempty" gorm:"type:varchar(36)"`
	Mode                string     `json:"mode" gorm:"type:varchar(32);not null"`
	Provider            string     `json:"provider" gorm:"type:varchar(50);not null;default:''"`
	Model               string     `json:"model" gorm:"type:varchar(255);not null;default:''"`
	Status              string     `json:"status" gorm:"type:varchar(32);not null;index"`
	Reason              string     `json:"reason,omitempty" gorm:"type:varchar(64);not null;default:''"`
	Detail              string     `json:"detail,omitempty" gorm:"type:text"`
	FinishReason        string     `json:"finish_reason,omitempty" gorm:"type:varchar(64);not null;default:''"`
	InputTokensEstimate int        `json:"input_tokens_estimate" gorm:"not null;default:0"`
	InputTokenBudget    int        `json:"input_token_budget" gorm:"not null;default:0"`
	ContextTokens       int        `json:"context_tokens" gorm:"not null;default:0"`
	MaxOutputTokens     int        `json:"max_output_tokens" gorm:"not null;default:0"`
	PromptTokens        int        `json:"prompt_tokens" gorm:"not null;default:0"`
	OutputTokens        int        `json:"output_tokens" gorm:"not null;default:0"`
	OutputChars         int        `json:"output_chars" gorm:"not null;default:0"`
	SummaryID           *string    `json:"summary_id,omitempty" gorm:"type:varchar(36)"`
	StartedAt           time.Time  `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate ensures SummaryAttempt has a UUID primary key
func (a *SummaryAttempt) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// BeforeCreate ensures Summary has a UUID primary key
func (s *Summary) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}
