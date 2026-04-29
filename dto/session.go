package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── Session DTOs ─────────────────────────────────────────────────────────────

type StartSessionRequest struct {
	PersonaID *uuid.UUID `json:"persona_id"`
}

type EndSessionRequest struct{}

type SessionResponse struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	PersonaID *string `json:"persona_id,omitempty"`
	Status    string  `json:"status"`

	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`

	PersonaName  string `json:"persona_name,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	DurationSecs int    `json:"duration_secs,omitempty"`

	// Report & Proctoring
	AIReport        string `json:"ai_report,omitempty"`
	MultipleFaces   bool   `json:"multiple_faces"`
	TabSwitchCount  int    `json:"tab_switch_count"`
	SuspiciousAudio bool   `json:"suspicious_audio"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── Chat / Message DTOs ──────────────────────────────────────────────────────

type SendMessageRequest struct {
	Content string `json:"content" validate:"required,min=1,max=8000"`
}

type MessageResponse struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	SequenceNum int       `json:"sequence_num"`
	CreatedAt   time.Time `json:"created_at"`
}

type ChatHistoryResponse struct {
	SessionID string            `json:"session_id"`
	Messages  []MessageResponse `json:"messages"`
}

// ─── SSE event shapes ─────────────────────────────────────────────────────────

type SSEEvent struct {
	Type    SSEEventType `json:"type"`
	Payload interface{}  `json:"payload"`
}

type SSEEventType string

const (
	SSEEventTypeDelta SSEEventType = "delta"
	SSEEventTypeDone  SSEEventType = "done"
	SSEEventTypeError SSEEventType = "error"
)

type SSEDeltaPayload struct {
	Content string `json:"content"`
}

type SSEErrorPayload struct {
	Message string `json:"message"`
}

// ─── UUID helpers ─────────────────────────────────────────────────────────────

// uuidToString converts a uuid.UUID to its canonical string form.
func UUIDToString(id uuid.UUID) string {
	return id.String()
}

// UUIDPtrToStringPtr safely converts *uuid.UUID → *string.
func UUIDPtrToStringPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}
