package main

import (
	"flag"
	"log"
	"os"

	"github.com/dhrruvsharma/skill-charge-backend/database"
	"github.com/dhrruvsharma/skill-charge-backend/routes"
	"github.com/dhrruvsharma/skill-charge-backend/services"
	"github.com/gin-contrib/cors"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	migrate := flag.Bool("migrate", false, "run database migrations and exit")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] no .env file found, relying on environment variables")
	}

	database.Connect()
	db := database.GetDB()
	deepseekSvc := services.NewDeepseekService(os.Getenv("DEEPSEEK_API_KEY"))
	voiceSvc := services.NewVoiceService(os.Getenv("ELEVENLABS_API_KEY"), os.Getenv("ELEVENLABS_VOICE_ID"))

	videoStoragePath := os.Getenv("VIDEO_STORAGE_PATH")
	if videoStoragePath == "" {
		videoStoragePath = "./recordings"
	}
	videoSvc, err := services.NewVideoService(videoStoragePath, voiceSvc)
	if err != nil {
		log.Fatalf("failed to init video service: %v", err)
	}

	if *migrate {
		database.RunMigrations("./migrations")
		log.Println("migrations complete, exiting")
		return
	}

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	routes.Register(r, db, deepseekSvc, voiceSvc, videoSvc)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("server starting on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
