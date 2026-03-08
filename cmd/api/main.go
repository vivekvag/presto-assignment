package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"presto/internal/api"
	"presto/internal/config"
	"presto/internal/database"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}

	handler := api.NewHandler(db)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(handler),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("server listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
