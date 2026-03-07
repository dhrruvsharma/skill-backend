package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageRole string
type SessionStatus string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"

	SessionStatusPending   SessionStatus = "pending"
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusAbandoned SessionStatus = "abandoned"
)

// InterviewMessage stores each turn in the interview conversation.
type InterviewMessage struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime"                                 json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"                                 json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	SessionID uuid.UUID        `gorm:"type:uuid;not null;index"  json:"session_id"`
	Session   InterviewSession `gorm:"foreignKey:SessionID"      json:"-"`

	Role    MessageRole `gorm:"type:varchar(20);not null"  json:"role"`    // user | assistant | system
	Content string      `gorm:"type:text;not null"         json:"content"` // raw message text

	// Token tracking per message for cost awareness
	PromptTokens     int `gorm:"default:0" json:"prompt_tokens,omitempty"`
	CompletionTokens int `gorm:"default:0" json:"completion_tokens,omitempty"`

	// Ordering — explicit sequence number within the session
	SequenceNum int `gorm:"not null;default:0" json:"sequence_num"`
}

func (m *InterviewMessage) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
