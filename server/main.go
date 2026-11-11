package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"screentime-server/api"
	"screentime-server/db"
)

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	dbPath := flag.String("db", "screentime.db", "Path to SQLite database file")
	staticDir := flag.String("static", "../web/dist/web/browser", "Path to web static directory")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		*dbPath = envDB
	}
	if envStatic := os.Getenv("STATIC_DIR"); envStatic != "" {
		*staticDir = envStatic
	}

	// Fallback check if running from repo root
	if _, err := os.Stat(*staticDir); err != nil {
		if _, err := os.Stat("web/dist/web/browser"); err == nil {
			*staticDir = "web/dist/web/browser"
		}
	}

	log.Println("Starting Screen Time Management Server...")
	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Fatal: Database initialization error: %v", err)
	}
	defer database.Close()

	server := api.NewServer(database, *staticDir)

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	log.Printf("Screen Time Server running on http://%s (serving static from: %s)", addr, *staticDir)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

