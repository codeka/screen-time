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
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		*dbPath = envDB
	}

	log.Println("Starting Screen Time Management Server...")
	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Fatal: Database initialization error: %v", err)
	}
	defer database.Close()

	server := api.NewServer(database)

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	log.Printf("Screen Time Server running on http://%s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

