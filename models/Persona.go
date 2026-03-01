package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DifficultyLevel string
type InterviewDomain string

const (
	DifficultyEasy   DifficultyLevel = "easy"
	DifficultyMedium DifficultyLevel = "medium"
	DifficultyHard   DifficultyLevel = "hard"

	DomainSoftwareEngineering InterviewDomain = "software_engineering"
	DomainDataScience         InterviewDomain = "data_science"
	DomainProductManagement   InterviewDomain = "product_management"
	DomainDesign              InterviewDomain = "design"
	DomainMarketing           InterviewDomain = "marketing"
	DomainFinance             InterviewDomain = "finance"
	DomainGeneral             InterviewDomain = "general"
)

// Persona represents an interview persona/role the user is practicing for.
// Each user can have multiple personas (e.g., "Google SWE L5", "Startup PM").
type Persona struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime"                                 json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"                                 json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	// FK to User
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	User   User      `gorm:"foreignKey:UserID"        json:"-"` // back-reference, omit in JSON to avoid cycles

	// Persona Identity
	Name        string `gorm:"type:varchar(150);not null"  json:"name"`        // e.g. "Senior SWE at Google"
	Description string `gorm:"type:text"                   json:"description"` // user-written context about this persona
	IsDefault   bool   `gorm:"default:false"               json:"is_default"`  // one persona can be the user's default
	IsActive    bool   `gorm:"default:true"                json:"is_active"`

	// Role & Domain
	TargetRole      string          `gorm:"type:varchar(150)"                 json:"target_role"`      // e.g. "Software Engineer"
	TargetCompany   string          `gorm:"type:varchar(150)"                 json:"target_company"`   // e.g. "Google" (optional)
	ExperienceYears int             `gorm:"default:0"                         json:"experience_years"` // years of experience the persona assumes
	Domain          InterviewDomain `gorm:"type:varchar(100);default:'general'" json:"domain"`
	Difficulty      DifficultyLevel `gorm:"type:varchar(50);default:'medium'"   json:"difficulty"`

	// Skills & Focus Areas — stored as a JSON array, e.g. ["Go", "System Design", "Kubernetes"]
	Skills datatypes.JSON `gorm:"type:jsonb" json:"skills"`

	// AI Prompt Context — this is injected as the system prompt when this persona is active
	// It tells the AI how to behave, what kind of questions to ask, what to evaluate, etc.
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`

	// Interview Settings
	InterviewDurationMins int  `gorm:"default:30"    json:"interview_duration_mins"`
	EnableVideoProctoring bool `gorm:"default:false" json:"enable_video_proctoring"`
	EnableAudioProctoring bool `gorm:"default:false" json:"enable_audio_proctoring"`
	EnableTabDetection    bool `gorm:"default:true"  json:"enable_tab_detection"`

	// Sessions linked to this persona
	Sessions []InterviewSession `gorm:"foreignKey:PersonaID;constraint:OnDelete:SET NULL" json:"sessions,omitempty"`
}

// InterviewSession tracks each interview attempt under a persona.
// Kept here to keep models co-located; move to session.go if it grows large.
type InterviewSession struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime"                                 json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"                                 json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"          json:"user_id"`
	PersonaID *uuid.UUID `gorm:"type:uuid;index"                   json:"persona_id"` // nullable — persona could be deleted
	Persona   *Persona   `gorm:"foreignKey:PersonaID"              json:"persona,omitempty"`

	// Session State
	StartedAt    *time.Time `gorm:"type:timestamp" json:"started_at"`
	EndedAt      *time.Time `gorm:"type:timestamp" json:"ended_at"`
	DurationSecs int        `gorm:"default:0"      json:"duration_secs"` // computed at end

	// Transcript & Report
	Transcript string `gorm:"type:text" json:"transcript,omitempty"` // full conversation log (text)
	AIReport   string `gorm:"type:text" json:"ai_report,omitempty"`  // AI-generated post-interview report

	// Cheating Flags
	TabSwitchCount  int            `gorm:"default:0"     json:"tab_switch_count"`
	SuspiciousAudio bool           `gorm:"default:false" json:"suspicious_audio"`
	MultipleFaces   bool           `gorm:"default:false" json:"multiple_faces"`
	CheatingFlags   datatypes.JSON `gorm:"type:jsonb"    json:"cheating_flags"` // detailed log of flagged events [{type, timestamp, detail}]

	// Recording
	RecordingURL string `gorm:"type:text" json:"recording_url,omitempty"` // S3/MinIO URL
}

func (p *Persona) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (s *InterviewSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
