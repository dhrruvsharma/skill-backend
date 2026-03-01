package database

import (
	"fmt"
	"log"
	"os"

	"github.com/dhrruvsharma/skill-charge-backend/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect() {
	dsn := fmt.Sprintf(
		os.Getenv("DATABASE_URL"),
	)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	log.Println("database connected successfully")
	migrate()
}

func migrate() {
	err := DB.AutoMigrate(
		&models.User{},
		&models.Persona{},
		&models.InterviewSession{},
	)
	if err != nil {
		log.Fatalf("auto migration failed: %v", err)
	}
	log.Println("database migration complete")
}

func GetDB() *gorm.DB {
	return DB
}
