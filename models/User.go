package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRole string
type AuthProvider string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"

	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
	ProviderGithub AuthProvider = "github"
)

type User struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime"                                 json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"                                 json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	FirstName string `gorm:"type:varchar(100);not null"      json:"first_name"`
	LastName  string `gorm:"type:varchar(100);not null"      json:"last_name"`
	Email     string `gorm:"type:varchar(255);uniqueIndex:idx_users_email;not null" json:"email"`

	PasswordHash string       `gorm:"type:text"                            json:"-"`
	Provider     AuthProvider `gorm:"type:varchar(50);default:'local'"     json:"provider"`
	ProviderID   string       `gorm:"type:varchar(255)"                    json:"provider_id,omitempty"`
	Role         UserRole     `gorm:"type:varchar(50);default:'user'"      json:"role"`
	IsVerified   bool         `gorm:"default:false"                        json:"is_verified"`
	IsActive     bool         `gorm:"default:true"                         json:"is_active"`

	ResetToken          string     `gorm:"type:varchar(255);index" json:"-"`
	ResetTokenExpiresAt *time.Time `gorm:"type:timestamp"          json:"-"`

	VerificationToken          string     `gorm:"type:varchar(255);index" json:"-"`
	VerificationTokenExpiresAt *time.Time `gorm:"type:timestamp"          json:"-"`

	RefreshTokenHash string     `gorm:"type:text"          json:"-"`
	RefreshTokenExp  *time.Time `gorm:"type:timestamp"     json:"-"`

	Personas []Persona `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"personas,omitempty"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
