package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/dhrruvsharma/skill-charge-backend/models"
)

var (
	ErrSessionNotFound     = errors.New("interview session not found")
	ErrSessionNotActive    = errors.New("interview session is not active")
	ErrSessionAlreadyEnded = errors.New("interview session has already ended")
	ErrPersonaNotFound     = errors.New("persona not found")
	ErrUnauthorized        = errors.New("unauthorized")
)

type InterviewService struct {
	db       *gorm.DB
	deepseek *DeepseekService
}

func NewInterviewService(db *gorm.DB, deepseek *DeepseekService) *InterviewService {
	return &InterviewService{db: db, deepseek: deepseek}
}

// ─── Session lifecycle ────────────────────────────────────────────────────────

// StartSession creates a new InterviewSession for the given user.
// If personaID is nil it falls back to the user's default persona (if any).
func (s *InterviewService) StartSession(
	ctx context.Context,
	userID uuid.UUID,
	personaID *uuid.UUID,
) (*models.InterviewSession, error) {

	// Resolve persona
	var persona *models.Persona
	if personaID != nil {
		var p models.Persona
		err := s.db.WithContext(ctx).
			Where("id = ? AND user_id = ?", *personaID, userID).
			First(&p).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrPersonaNotFound
			}
			return nil, fmt.Errorf("start session: fetch persona: %w", err)
		}
		persona = &p
	} else {
		// try default persona
		var p models.Persona
		err := s.db.WithContext(ctx).
			Where("user_id = ? AND is_default = true AND is_active = true", userID).
			First(&p).Error
		if err == nil {
			persona = &p
		}
		// If no default persona exists, session runs without one (generic interview)
	}

	now := time.Now()
	session := &models.InterviewSession{
		UserID:    userID,
		Status:    models.SessionStatus(string(models.SessionStatusActive)),
		StartedAt: &now,
	}
	if persona != nil {
		session.PersonaID = &persona.ID
	}

	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, fmt.Errorf("start session: create: %w", err)
	}

	// Persist the opening system message so history is always complete.
	systemPrompt := s.resolveSystemPrompt(persona)
	if systemPrompt != "" {
		msg := &models.InterviewMessage{
			SessionID:   session.ID,
			Role:        models.MessageRoleSystem,
			Content:     systemPrompt,
			SequenceNum: 0,
		}
		if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
			return nil, fmt.Errorf("start session: create system message: %w", err)
		}
	}

	// Generate AI greeting message (best-effort — don't fail session creation)
	if s.deepseek != nil {
		aiMessages := []AIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "[SESSION_START] The candidate has just joined the interview. Introduce yourself briefly based on the persona/role context and ask your first question."},
		}
		ch, err := s.deepseek.StreamChat(ctx, aiMessages)
		if err == nil {
			var greeting strings.Builder
			var promptTokens, completionTokens int
			for chunk := range ch {
				if chunk.Err != nil {
					break
				}
				if chunk.Done {
					promptTokens = chunk.PromptTokens
					completionTokens = chunk.CompletionTokens
					break
				}
				greeting.WriteString(chunk.Content)
			}
			if greeting.Len() > 0 {
				greetingMsg := &models.InterviewMessage{
					SessionID:        session.ID,
					Role:             models.MessageRoleAssistant,
					Content:          greeting.String(),
					SequenceNum:      1,
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
				}
				_ = s.db.WithContext(ctx).Create(greetingMsg).Error
			}
		}
	}

	// Reload with persona for the response
	if err := s.db.WithContext(ctx).
		Preload("Persona").
		First(session, "id = ?", session.ID).Error; err != nil {
		return nil, fmt.Errorf("start session: reload: %w", err)
	}

	return session, nil
}

// EndSession marks the session as completed and calculates duration.
func (s *InterviewService) EndSession(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (*models.InterviewSession, error) {

	session, err := s.getOwnedSession(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if session.Status == models.SessionStatusCompleted ||
		session.Status == models.SessionStatusAbandoned {
		return nil, ErrSessionAlreadyEnded
	}

	now := time.Now()
	var durationSecs int
	if session.StartedAt != nil {
		durationSecs = int(now.Sub(*session.StartedAt).Seconds())
	}

	if err := s.db.WithContext(ctx).Model(session).Updates(map[string]interface{}{
		"status":        string(models.SessionStatusCompleted),
		"ended_at":      now,
		"duration_secs": durationSecs,
	}).Error; err != nil {
		return nil, fmt.Errorf("end session: update: %w", err)
	}

	session.Status = models.SessionStatusCompleted
	session.EndedAt = &now
	session.DurationSecs = durationSecs
	return session, nil
}

// GetSession returns a session owned by userID.
func (s *InterviewService) GetSession(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (*models.InterviewSession, error) {
	return s.getOwnedSession(ctx, sessionID, userID)
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

// GetHistory returns ordered messages for a session.
func (s *InterviewService) GetHistory(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) ([]models.InterviewMessage, error) {

	// Ownership check
	if _, err := s.getOwnedSession(ctx, sessionID, userID); err != nil {
		return nil, err
	}

	var msgs []models.InterviewMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence_num ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return msgs, nil
}

// nextSequenceNum returns the next sequence number for a session (thread-safe within a request).
func (s *InterviewService) nextSequenceNum(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var max int
	err := s.db.WithContext(ctx).
		Model(&models.InterviewMessage{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(sequence_num), -1)").
		Scan(&max).Error
	if err != nil {
		return 0, fmt.Errorf("next sequence: %w", err)
	}
	return max + 1, nil
}

// SendMessageStream persists the user message, builds the prompt history, calls
// Deepseek with streaming, and returns the chunk channel.
// The caller is responsible for reading all chunks and then calling
// FinalizeAssistantMessage to persist the completed assistant turn.
func (s *InterviewService) SendMessageStream(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
	content string,
) (*models.InterviewMessage, <-chan StreamChunk, error) {

	session, err := s.getOwnedSession(ctx, sessionID, userID)
	if err != nil {
		return nil, nil, err
	}
	if session.Status != models.SessionStatusActive {
		return nil, nil, ErrSessionNotActive
	}

	// Persist user message
	seq, err := s.nextSequenceNum(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	userMsg := &models.InterviewMessage{
		SessionID:   sessionID,
		Role:        models.MessageRoleUser,
		Content:     content,
		SequenceNum: seq,
	}
	if err := s.db.WithContext(ctx).Create(userMsg).Error; err != nil {
		return nil, nil, fmt.Errorf("send message: save user msg: %w", err)
	}

	// Build full AI message history
	aiMessages, err := s.buildAIMessages(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	ch, err := s.deepseek.StreamChat(ctx, aiMessages)
	if err != nil {
		return nil, nil, fmt.Errorf("send message: deepseek stream: %w", err)
	}

	return userMsg, ch, nil
}

// FinalizeAssistantMessage persists the assembled assistant reply after the
// stream completes. Returns the saved message.
func (s *InterviewService) FinalizeAssistantMessage(
	ctx context.Context,
	sessionID uuid.UUID,
	content string,
	promptTokens int,
	completionTokens int,
) (*models.InterviewMessage, error) {

	seq, err := s.nextSequenceNum(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	msg := &models.InterviewMessage{
		SessionID:        sessionID,
		Role:             models.MessageRoleAssistant,
		Content:          content,
		SequenceNum:      seq,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, fmt.Errorf("finalize assistant msg: %w", err)
	}
	return msg, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *InterviewService) getOwnedSession(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (*models.InterviewSession, error) {

	var session models.InterviewSession
	err := s.db.WithContext(ctx).
		Preload("Persona").
		Where("id = ? AND user_id = ?", sessionID, userID).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &session, nil
}

func (s *InterviewService) buildAIMessages(
	ctx context.Context,
	sessionID uuid.UUID,
) ([]AIMessage, error) {

	var msgs []models.InterviewMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence_num ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("build ai messages: %w", err)
	}

	result := make([]AIMessage, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, AIMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}
	return result, nil
}

func (s *InterviewService) resolveSystemPrompt(persona *models.Persona) string {
	base := `You are a professional technical interviewer.
Conduct a structured interview by asking one clear question at a time.
Evaluate answers on correctness, clarity, and depth.
Be encouraging but objective. After each answer, provide brief feedback before moving to the next question.`

	if persona != nil && persona.SystemPrompt != "" {
		base = persona.SystemPrompt
	}

	// Append the interview-end instruction to every system prompt
	base += `

IMPORTANT: When you have asked enough questions (typically 5-8 questions) and gathered sufficient information to evaluate the candidate, you MUST end the interview. To do so, provide a natural closing statement thanking the candidate, and then include the exact marker [END_INTERVIEW] at the very end of your message. Do NOT include this marker until you are ready to conclude the interview. The marker must appear on its own line at the end.`

	return base
}

// UpdateSessionRecording saves the video path and face-detection result on the
// session. If multiple faces were found it also appends a cheating_flags event.
func (s *InterviewService) UpdateSessionRecording(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
	recordingPath string,
	multipleFaces bool,
	maxFaces int,
) error {
	session, err := s.getOwnedSession(ctx, sessionID, userID)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{
		"recording_url":  recordingPath,
		"multiple_faces": multipleFaces,
	}

	if multipleFaces {
		type flagEvent struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Detail    string    `json:"detail"`
		}
		var existing []flagEvent
		if len(session.CheatingFlags) > 0 {
			_ = json.Unmarshal(session.CheatingFlags, &existing)
		}
		existing = append(existing, flagEvent{
			Type:      "multiple_faces",
			Timestamp: time.Now().UTC(),
			Detail:    fmt.Sprintf("maximum %d faces detected in video", maxFaces),
		})
		flagsJSON, err := json.Marshal(existing)
		if err == nil {
			updates["cheating_flags"] = datatypes.JSON(flagsJSON)
		}
	}

	return s.db.WithContext(ctx).Model(session).Updates(updates).Error
}

// GenerateReport creates an AI-generated interview report based on the full
// conversation transcript and proctoring data. Saves it to session.AIReport.
func (s *InterviewService) GenerateReport(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (string, error) {

	session, err := s.getOwnedSession(ctx, sessionID, userID)
	if err != nil {
		return "", err
	}

	// Build transcript text
	msgs, err := s.GetHistory(ctx, sessionID, userID)
	if err != nil {
		return "", err
	}

	var transcriptBuilder strings.Builder
	for _, m := range msgs {
		if m.Role == models.MessageRoleSystem {
			continue
		}
		role := "Candidate"
		if m.Role == models.MessageRoleAssistant {
			role = "Interviewer"
		}
		transcriptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, m.Content))
	}

	// Build proctoring summary
	var proctoringInfo strings.Builder
	proctoringInfo.WriteString(fmt.Sprintf("- Multiple faces detected: %v\n", session.MultipleFaces))
	proctoringInfo.WriteString(fmt.Sprintf("- Tab switches: %d\n", session.TabSwitchCount))
	proctoringInfo.WriteString(fmt.Sprintf("- Suspicious audio: %v\n", session.SuspiciousAudio))
	if len(session.CheatingFlags) > 0 {
		proctoringInfo.WriteString(fmt.Sprintf("- Cheating flags: %s\n", string(session.CheatingFlags)))
	}

	reportPrompt := fmt.Sprintf(`You are an expert interview evaluator. Based on the following interview transcript, generate a detailed performance report.

## Interview Transcript
%s

## Proctoring Data
%s

Generate a JSON report with this exact structure (no markdown code fences, just raw JSON):
{
  "overall_score": <number 1-10>,
  "summary": "<2-3 sentence overall assessment>",
  "strengths": ["<strength 1>", "<strength 2>", ...],
  "weaknesses": ["<weakness 1>", "<weakness 2>", ...],
  "detailed_feedback": "<paragraph with specific actionable advice>",
  "question_scores": [
    {"question": "<brief question topic>", "score": <1-10>, "feedback": "<one line>"}
  ],
  "proctoring": {
    "multiple_faces_detected": <bool>,
    "tab_switch_count": <number>,
    "suspicious_audio": <bool>,
    "integrity_note": "<one sentence about proctoring observations or 'No issues detected'>"
  }
}`, transcriptBuilder.String(), proctoringInfo.String())

	aiMessages := []AIMessage{
		{Role: "system", Content: "You are an interview performance evaluator. Return ONLY valid JSON, no explanations or code fences."},
		{Role: "user", Content: reportPrompt},
	}

	ch, err := s.deepseek.StreamChat(ctx, aiMessages)
	if err != nil {
		return "", fmt.Errorf("generate report: deepseek: %w", err)
	}

	var reportBuilder strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return "", fmt.Errorf("generate report: stream error: %w", chunk.Err)
		}
		if chunk.Done {
			break
		}
		reportBuilder.WriteString(chunk.Content)
	}

	report := reportBuilder.String()

	// Persist
	if err := s.db.WithContext(ctx).Model(session).Update("ai_report", report).Error; err != nil {
		return "", fmt.Errorf("generate report: save: %w", err)
	}

	return report, nil
}

// CompileTranscript creates a text transcript of the session (excluding system messages).
func (s *InterviewService) CompileTranscript(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (string, error) {
	msgs, err := s.GetHistory(ctx, sessionID, userID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, m := range msgs {
		if m.Role == models.MessageRoleSystem {
			continue
		}
		role := "User"
		if m.Role == models.MessageRoleAssistant {
			role = "AI"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, m.Content))
	}

	return sb.String(), nil
}

func (s *InterviewService) GetUserSessions(
	ctx context.Context,
	userID uuid.UUID,
) ([]models.InterviewSession, error) {

	var sessions []models.InterviewSession
	if err := s.db.WithContext(ctx).
		Preload("Persona").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("get user sessions: %w", err)
	}

	return sessions, nil
}
