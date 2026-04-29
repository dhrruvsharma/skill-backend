package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dhrruvsharma/skill-charge-backend/dto"
	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/models"
	"github.com/dhrruvsharma/skill-charge-backend/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ─── Session Handlers ─────────────────────────────────────────────────────────

// StartSession godoc
// POST /api/v1/sessions
func StartSession(db *gorm.DB, deepseekSvc *services.DeepseekService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		var req dto.StartSessionRequest
		// EOF is fine — empty body means no persona_id supplied
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}

		svc := services.NewInterviewService(db, deepseekSvc)
		session, err := svc.StartSession(c.Request.Context(), userID, req.PersonaID)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"success": true, "data": toSessionResponse(session)})
	}
}

// EndSession godoc
// PATCH /api/v1/sessions/:id/end
func EndSession(db *gorm.DB, deepseekSvc *services.DeepseekService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid session id"})
			return
		}

		svc := services.NewInterviewService(db, deepseekSvc)
		session, err := svc.EndSession(c.Request.Context(), sessionID, userID)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		// Generate report in the background — best effort
		report, _ := svc.GenerateReport(c.Request.Context(), sessionID, userID)
		if report != "" {
			session.AIReport = report
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": toSessionResponse(session)})
	}
}

// GetSession godoc
// GET /api/v1/sessions/:id
func GetSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid session id"})
			return
		}

		svc := services.NewInterviewService(db, nil) // deepseek not needed for reads
		session, err := svc.GetSession(c.Request.Context(), sessionID, userID)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": toSessionResponse(session)})
	}
}

// ─── Chat Handlers ────────────────────────────────────────────────────────────

// GetHistory godoc
// GET /api/v1/sessions/:id/messages
func GetHistory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid session id"})
			return
		}

		svc := services.NewInterviewService(db, nil)
		msgs, err := svc.GetHistory(c.Request.Context(), sessionID, userID)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		responses := make([]dto.MessageResponse, 0, len(msgs))
		for _, m := range msgs {
			if m.Role == models.MessageRoleSystem {
				continue // never expose system messages to the client
			}
			responses = append(responses, toMessageResponse(m))
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": dto.ChatHistoryResponse{
				SessionID: sessionID.String(),
				Messages:  responses,
			},
		})
	}
}

// SendMessage godoc
// POST /api/v1/sessions/:id/messages
//
// Streams the AI reply as SSE. Each `data:` line is a JSON dto.SSEEvent:
//
//	delta  → { "type": "delta", "payload": { "content": "..." } }
//	done   → { "type": "done",  "payload": <MessageResponse>    }
//	error  → { "type": "error", "payload": { "message": "..." } }
func SendMessage(db *gorm.DB, deepseekSvc *services.DeepseekService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid session id"})
			return
		}

		var req dto.SendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		req.Content = strings.TrimSpace(req.Content)
		if req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "content must not be empty"})
			return
		}

		svc := services.NewInterviewService(db, deepseekSvc)
		_, streamCh, err := svc.SendMessageStream(c.Request.Context(), sessionID, userID, req.Content)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		// ── SSE headers ──────────────────────────────────────────────────────
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		flusher, canFlush := c.Writer.(http.Flusher)

		writeEvent := func(evt dto.SSEEvent) {
			b, _ := json.Marshal(evt)
			fmt.Fprintf(c.Writer, "data: %s\n\n", string(b))
			if canFlush {
				flusher.Flush()
			}
		}

		// ── Drain stream ─────────────────────────────────────────────────────
		var (
			sb               strings.Builder
			promptTokens     int
			completionTokens int
		)

		ctx := c.Request.Context()

		for chunk := range streamCh {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if chunk.Err != nil {
				writeEvent(dto.SSEEvent{
					Type:    dto.SSEEventTypeError,
					Payload: dto.SSEErrorPayload{Message: chunk.Err.Error()},
				})
				return
			}

			if chunk.Done {
				promptTokens = chunk.PromptTokens
				completionTokens = chunk.CompletionTokens
				break
			}

			sb.WriteString(chunk.Content)
			writeEvent(dto.SSEEvent{
				Type:    dto.SSEEventTypeDelta,
				Payload: dto.SSEDeltaPayload{Content: chunk.Content},
			})
		}

		fullText := sb.String()

		// Check for AI-initiated interview end
		writeSSE := func(eventType, payload string) {
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, payload)
			if canFlush {
				flusher.Flush()
			}
		}
		cleanedText, ended := checkAndHandleEndInterview(ctx, svc, sessionID, userID, fullText, writeSSE)
		_ = ended

		// Persist completed assistant message
		assistantMsg, err := svc.FinalizeAssistantMessage(ctx, sessionID, cleanedText, promptTokens, completionTokens)
		if err != nil {
			writeEvent(dto.SSEEvent{
				Type:    dto.SSEEventTypeError,
				Payload: dto.SSEErrorPayload{Message: "failed to save assistant message"},
			})
			return
		}

		writeEvent(dto.SSEEvent{
			Type:    dto.SSEEventTypeDone,
			Payload: toMessageResponse(*assistantMsg),
		})
	}
}

// DeleteSession godoc
// DELETE /api/v1/sessions/:id
func DeleteSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid session id"})
			return
		}

		// Delete messages first, then session (soft-delete)
		if err := db.Where("session_id = ?", sessionID).Delete(&models.InterviewMessage{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete session messages"})
			return
		}

		result := db.Where("id = ? AND user_id = ?", sessionID, userID).Delete(&models.InterviewSession{})
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete session"})
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "session not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "session deleted successfully"})
	}
}

func GetUserSessions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		svc := services.NewInterviewService(db, nil)
		sessions, err := svc.GetUserSessions(c.Request.Context(), userID)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		responses := make([]dto.SessionResponse, 0, len(sessions))
		for _, s := range sessions {
			responses = append(responses, toSessionResponse(&s))
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": responses})
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func handleInterviewServiceError(c *gin.Context, err error) {
	switch err {
	case services.ErrSessionNotFound, services.ErrPersonaNotFound:
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
	case services.ErrUnauthorized:
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
	case services.ErrSessionNotActive, services.ErrSessionAlreadyEnded:
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "internal server error"})
	}
}

func toSessionResponse(s *models.InterviewSession) dto.SessionResponse {
	resp := dto.SessionResponse{
		ID:              s.ID.String(),
		UserID:          s.UserID.String(),
		PersonaID:       dto.UUIDPtrToStringPtr(s.PersonaID),
		Status:          string(s.Status),
		StartedAt:       s.StartedAt,
		EndedAt:         s.EndedAt,
		DurationSecs:    s.DurationSecs,
		AIReport:        s.AIReport,
		MultipleFaces:   s.MultipleFaces,
		TabSwitchCount:  s.TabSwitchCount,
		SuspiciousAudio: s.SuspiciousAudio,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.Persona != nil {
		resp.PersonaName = s.Persona.Name
		resp.SystemPrompt = s.Persona.SystemPrompt
	}
	return resp
}

func toMessageResponse(m models.InterviewMessage) dto.MessageResponse {
	return dto.MessageResponse{
		ID:          m.ID.String(),
		SessionID:   m.SessionID.String(),
		Role:        string(m.Role),
		Content:     m.Content,
		SequenceNum: m.SequenceNum,
		CreatedAt:   m.CreatedAt,
	}
}
