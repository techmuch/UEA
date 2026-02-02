package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/user/uea/internal/embed"
	"github.com/user/uea/internal/store"
)

func main() {
	fmt.Println("Hello, UEA!")

	// Initialize Database
	dataDir := filepath.Join(".", "data")
	_, err := store.InitDB(dataDir) // Assign to blank identifier
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.CloseDB()
	log.Printf("Database initialized successfully at %s/%s", dataDir, store.DBNAME)

	content, err := embed.Content()
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", http.FileServer(http.FS(content)))

	fmt.Println("Serving frontend on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
