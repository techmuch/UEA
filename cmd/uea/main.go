package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/user/uea/internal/account"
	"github.com/user/uea/internal/auth"
	"github.com/user/uea/internal/embed"
	"github.com/user/uea/internal/message"
	"github.com/user/uea/internal/store"
	"github.com/user/uea/internal/sync"
)

var syncManager = sync.NewSyncManager(5)

func main() {
	fmt.Println("Starting Universal Email Analytics (UEA)...")

	// Initialize Database
	dataDir := filepath.Join(".", "data")
	_, err := store.InitDB(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.CloseDB()
	log.Printf("Database initialized successfully at %s/%s", dataDir, store.DBNAME)

	// Create initial user for testing
	if err := auth.CreateInitialUser("admin@uea.local", "password123"); err != nil {
		log.Printf("Warning: failed to create initial user: %v", err)
	}

	// API Routes
	mux := http.NewServeMux()

	// Public API Routes
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)

	// Protected API Routes Sub-mux
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/accounts", handleAccounts)
	apiMux.HandleFunc("/api/accounts/stats", handleAccountStats)
	apiMux.HandleFunc("/api/accounts/sync", handleAccountSync)
	apiMux.HandleFunc("/api/messages", handleMessages)
	apiMux.HandleFunc("/api/message", handleMessage)
	apiMux.HandleFunc("/api/profile", handleProfile)
	apiMux.HandleFunc("/api/analytics", handleAnalytics)
	apiMux.HandleFunc("/api/settings", handleSettings)

	// Register the protected mux with auth middleware
	mux.Handle("/api/accounts", auth.Middleware(http.HandlerFunc(handleAccounts)))
	mux.Handle("/api/accounts/", auth.Middleware(apiMux))
	mux.Handle("/api/messages", auth.Middleware(http.HandlerFunc(handleMessages)))
	mux.Handle("/api/message", auth.Middleware(http.HandlerFunc(handleMessage)))
	mux.Handle("/api/profile", auth.Middleware(http.HandlerFunc(handleProfile)))
	mux.Handle("/api/analytics", auth.Middleware(http.HandlerFunc(handleAnalytics)))
	mux.Handle("/api/settings", auth.Middleware(http.HandlerFunc(handleSettings)))

	// Frontend Static Assets
	content, err := embed.Content()
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(content)))

	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		log.Printf("Login error: failed to decode JSON: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	session, err := auth.Authenticate(creds.Username, creds.Password)
	if err != nil {
		log.Printf("Login error: authentication failed for %s: %v", creds.Username, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	user, err := store.GetUserByID(session.UserID)
	if err != nil {
		log.Printf("Login error: failed to get user by ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		store.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	w.WriteHeader(http.StatusNoContent)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(auth.UserContextKey).(*store.User)

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)

	case http.MethodPost:
		var update store.User
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		user.DisplayName = update.DisplayName
		user.Email = update.Email
		user.ProfileImageURL = update.ProfileImageURL
		if err := store.SaveUser(user); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		accounts, err := store.ListAccounts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(accounts)

	case http.MethodPost:
		var acc account.Account
		if err := json.NewDecoder(r.Body).Decode(&acc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if acc.ID == "" {
			acc.ID = strings.ToLower(strings.ReplaceAll(acc.Name, " ", "-"))
		}
		if err := store.SaveAccount(&acc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(acc)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id parameter", http.StatusBadRequest)
			return
		}
		if err := store.DeleteAccount(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAccountStats(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	stats, err := store.GetAccountStats(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleAccountSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	acc, err := store.GetAccount(id)
	if err != nil || acc == nil {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	go syncManager.StartSync(acc)

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, "Sync started")
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("accountId")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	
	filter := store.AnalyticsFilter{
		Date:  r.URL.Query().Get("date"),
		From:  r.URL.Query().Get("from"),
		Topic: r.URL.Query().Get("topic"),
	}

	limit := 50
	offset := 0

	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}

	msgs, err := store.ListMessagesFiltered(accountID, filter, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*message.Message{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
// ... (no changes needed to handleMessage)
}

func handleAnalytics(w http.ResponseWriter, r *http.Request) {
	queryType := r.URL.Query().Get("type")
	filter := store.AnalyticsFilter{
		Date:  r.URL.Query().Get("date"),
		From:  r.URL.Query().Get("from"),
		Topic: r.URL.Query().Get("topic"),
	}
	var data interface{}
	var err error

	switch queryType {
	case "volume":
		data, err = store.GetTemporalVolume(filter)
	case "senders":
		data, err = store.GetTopSenders(filter)
	case "topics":
		data, err = store.GetTopicStats(filter)
	default:
		http.Error(w, "invalid analytics type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		val, err := store.GetSetting(key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": val})

	case http.MethodPost:
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpdateSetting(req.Key, req.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
