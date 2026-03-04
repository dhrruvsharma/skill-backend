package dto

import (
	"github.com/dhrruvsharma/skill-charge-backend/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type CreatePersonaRequest struct {
	Name        string `json:"name"                    binding:"required,min=1,max=150"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	IsActive    *bool  `json:"is_active"`

	// Role & Domain
	TargetRole      string                 `json:"target_role"       binding:"omitempty,max=150"`
	ExperienceYears int                    `json:"experience_years"  binding:"omitempty,min=0,max=60"`
	Domain          models.InterviewDomain `json:"domain"            binding:"omitempty,oneof=software_engineering data_science product_management design marketing finance general"`
	Difficulty      models.DifficultyLevel `json:"difficulty"        binding:"omitempty,oneof=easy medium hard"`

	Skills datatypes.JSON `json:"skills"`

	SystemPrompt string `json:"system_prompt"`

	InterviewDurationMins int  `json:"interview_duration_mins" binding:"omitempty,min=5,max=180"`
	EnableVideoProctoring bool `json:"enable_video_proctoring"`
	EnableAudioProctoring bool `json:"enable_audio_proctoring"`
	EnableTabDetection    bool `json:"enable_tab_detection"`
}

type UpdatePersonaRequest struct {
	Name        *string `json:"name"        binding:"omitempty,min=1,max=150"`
	Description *string `json:"description"`
	IsDefault   *bool   `json:"is_default"`
	IsActive    *bool   `json:"is_active"`

	TargetRole      *string                 `json:"target_role"      binding:"omitempty,max=150"`
	ExperienceYears *int                    `json:"experience_years" binding:"omitempty,min=0,max=60"`
	Domain          *models.InterviewDomain `json:"domain"           binding:"omitempty,oneof=software_engineering data_science product_management design marketing finance general"`
	Difficulty      *models.DifficultyLevel `json:"difficulty"       binding:"omitempty,oneof=easy medium hard"`

	Skills *datatypes.JSON `json:"skills"`

	SystemPrompt *string `json:"system_prompt"`

	InterviewDurationMins *int  `json:"interview_duration_mins" binding:"omitempty,min=5,max=180"`
	EnableVideoProctoring *bool `json:"enable_video_proctoring"`
	EnableAudioProctoring *bool `json:"enable_audio_proctoring"`
	EnableTabDetection    *bool `json:"enable_tab_detection"`
}

func (r *UpdatePersonaRequest) ToUpdateMap() map[string]interface{} {
	m := make(map[string]interface{})

	if r.Name != nil {
		m["name"] = *r.Name
	}
	if r.Description != nil {
		m["description"] = *r.Description
	}
	if r.IsDefault != nil {
		m["is_default"] = *r.IsDefault
	}
	if r.IsActive != nil {
		m["is_active"] = *r.IsActive
	}
	if r.TargetRole != nil {
		m["target_role"] = *r.TargetRole
	}
	if r.ExperienceYears != nil {
		m["experience_years"] = *r.ExperienceYears
	}
	if r.Domain != nil {
		m["domain"] = *r.Domain
	}
	if r.Difficulty != nil {
		m["difficulty"] = *r.Difficulty
	}
	if r.Skills != nil {
		m["skills"] = *r.Skills
	}
	if r.SystemPrompt != nil {
		m["system_prompt"] = *r.SystemPrompt
	}
	if r.InterviewDurationMins != nil {
		m["interview_duration_mins"] = *r.InterviewDurationMins
	}
	if r.EnableVideoProctoring != nil {
		m["enable_video_proctoring"] = *r.EnableVideoProctoring
	}
	if r.EnableAudioProctoring != nil {
		m["enable_audio_proctoring"] = *r.EnableAudioProctoring
	}
	if r.EnableTabDetection != nil {
		m["enable_tab_detection"] = *r.EnableTabDetection
	}

	return m
}

type PersonaResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	IsActive    bool      `json:"is_active"`

	TargetRole      string                 `json:"target_role"`
	ExperienceYears int                    `json:"experience_years"`
	Domain          models.InterviewDomain `json:"domain"`
	Difficulty      models.DifficultyLevel `json:"difficulty"`

	Skills       datatypes.JSON `json:"skills"`
	SystemPrompt string         `json:"system_prompt"`

	InterviewDurationMins int  `json:"interview_duration_mins"`
	EnableVideoProctoring bool `json:"enable_video_proctoring"`
	EnableAudioProctoring bool `json:"enable_audio_proctoring"`
	EnableTabDetection    bool `json:"enable_tab_detection"`

	CreatedAt string `json:"created_at"` // ISO-8601
	UpdatedAt string `json:"updated_at"`
}

func FromPersona(p models.Persona) PersonaResponse {
	return PersonaResponse{
		ID:                    p.ID,
		UserID:                p.UserID,
		Name:                  p.Name,
		Description:           p.Description,
		IsDefault:             p.IsDefault,
		IsActive:              p.IsActive,
		TargetRole:            p.TargetRole,
		ExperienceYears:       p.ExperienceYears,
		Domain:                p.Domain,
		Difficulty:            p.Difficulty,
		Skills:                p.Skills,
		SystemPrompt:          p.SystemPrompt,
		InterviewDurationMins: p.InterviewDurationMins,
		EnableVideoProctoring: p.EnableVideoProctoring,
		EnableAudioProctoring: p.EnableAudioProctoring,
		EnableTabDetection:    p.EnableTabDetection,
		CreatedAt:             p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:             p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func FromPersonaList(personas []models.Persona) []PersonaResponse {
	out := make([]PersonaResponse, 0, len(personas))
	for _, p := range personas {
		out = append(out, FromPersona(p))
	}
	return out
}
