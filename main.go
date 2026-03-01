package main

import (
	"log"
	"os"

	"github.com/dhrruvsharma/skill-charge-backend/database"
	"github.com/dhrruvsharma/skill-charge-backend/routes"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] no .env file found, relying on environment variables")
	}

	database.Connect()

	r := gin.Default()
	routes.Register(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("server starting on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
