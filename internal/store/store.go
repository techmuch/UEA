package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/user/uea/internal/account"
	"github.com/user/uea/internal/message"
)

const (
	// DBNAME is the default name for the SQLite database file.
	DBNAME = "uea.db"
	// SchemaVersion is the current version of the database schema.
	SchemaVersion = 8
)

var (
	db     *sql.DB
	dbOnce sync.Once
)

// Agent represents an AI Agent configuration.
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SchemaJSON  string `json:"schemaJson"`
}

// User represents a system user.
type User struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	PasswordHash    string `json:"-"`
	DisplayName     string `json:"displayName"`
	Email           string `json:"email"`
	ProfileImageURL string `json:"profileImageUrl"`
}

// Session represents a user session.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// MailboxSyncState represents the synchronization state for a specific mailbox.
type MailboxSyncState struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	LastUID   uint32 `json:"lastUid"`
	LastMODSEQ uint64 `json:"lastModseq"`
}

// AnalyticsData represents a point in a time series.
type AnalyticsData struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// AnalyticsFilter represents optional filters for analytics queries.
type AnalyticsFilter struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	From  string `json:"from"`  // email address
	Topic string `json:"topic"` // keyword
}

// InitDB initializes the SQLite database connection and sets up the schema.
func InitDB(dataDir string) (*sql.DB, error) {
	var err error
	dbOnce.Do(func() {
		dbPath := filepath.Join(dataDir, DBNAME)
		log.Printf("Initializing database at: %s", dbPath)

		if err = os.MkdirAll(dataDir, 0755); err != nil {
			err = fmt.Errorf("failed to create data directory: %w", err)
			return
		}

		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			err = fmt.Errorf("failed to open database: %w", err)
			return
		}

		_, err = db.Exec("PRAGMA journal_mode=WAL;")
		if err != nil {
			err = fmt.Errorf("failed to set WAL journal mode: %w", err)
			return
		}
		_, err = db.Exec("PRAGMA synchronous=NORMAL;")
		if err != nil {
			err = fmt.Errorf("failed to set synchronous mode: %w", err)
			return
		}

		err = migrateDB(db)
		if err != nil {
			err = fmt.Errorf("failed to run database migrations: %w", err)
			return
		}
	})

	return db, err
}

// migrateDB runs database migrations.
func migrateDB(db *sql.DB) error {
	var currentVersion int
	row := db.QueryRow("PRAGMA user_version;")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	log.Printf("Current database schema version: %d", currentVersion)

	if currentVersion < 1 {
		log.Println("Applying schema migration v1...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS accounts (
				id TEXT PRIMARY KEY,
				host TEXT NOT NULL,
				port INTEGER NOT NULL,
				user TEXT NOT NULL,
				password TEXT NOT NULL,
				ssl BOOLEAN NOT NULL
			);

			CREATE TABLE IF NOT EXISTS mailboxes (
				id TEXT PRIMARY KEY,
				account_id TEXT NOT NULL,
				name TEXT NOT NULL,
				last_uid INTEGER DEFAULT 0,
				last_modseq INTEGER DEFAULT 0,
				FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
				UNIQUE(account_id, name)
			);

			CREATE INDEX IF NOT EXISTS idx_mailboxes_account_id ON mailboxes(account_id);
		`)
		if err != nil {
			return fmt.Errorf("failed to apply schema v1: %w", err)
		}
		_, err = db.Exec("PRAGMA user_version = 1;")
		if err != nil {
			return err
		}
		currentVersion = 1
	}

	if currentVersion < 2 {
		log.Println("Applying schema migration v2 (messages table)...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS messages (
				id TEXT PRIMARY KEY,
				account_id TEXT NOT NULL,
				uid INTEGER NOT NULL,
				message_id TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				normalized_body TEXT NOT NULL,
				from_addr TEXT NOT NULL,
				to_addrs TEXT NOT NULL,
				cc_addrs TEXT NOT NULL,
				bcc_addrs TEXT NOT NULL,
				subject TEXT NOT NULL,
				date INTEGER NOT NULL,
				body TEXT NOT NULL,
				html_body TEXT NOT NULL,
				header BLOB NOT NULL,
				flags TEXT NOT NULL,
				size INTEGER NOT NULL,
				internal_date INTEGER NOT NULL,
				FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
			);

			CREATE INDEX IF NOT EXISTS idx_messages_account_id ON messages(account_id);
			CREATE INDEX IF NOT EXISTS idx_messages_message_id ON messages(message_id);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_content_hash ON messages(content_hash);
		`)
		if err != nil {
			return fmt.Errorf("failed to apply schema v2: %w", err)
		}
		_, err = db.Exec("PRAGMA user_version = 2;")
		if err != nil {
			return err
		}
		currentVersion = 2
	}

	if currentVersion < 3 {
		log.Println("Applying schema migration v3...")
		_, err := db.Exec(`
			ALTER TABLE accounts ADD COLUMN name TEXT;
			ALTER TABLE accounts ADD COLUMN smtp_host TEXT;
			ALTER TABLE accounts ADD COLUMN smtp_port INTEGER;
		`)
		if err != nil {
			log.Printf("Warning v3: %v", err)
		}
		_, err = db.Exec("PRAGMA user_version = 3;")
		if err != nil {
			return err
		}
		currentVersion = 3
	}

	if currentVersion < 4 {
		log.Println("Applying schema migration v4...")
		_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN email TEXT;`)
		if err != nil {
			log.Printf("Warning v4: %v", err)
		}
		_, err = db.Exec("PRAGMA user_version = 4;")
		if err != nil {
			return err
		}
		currentVersion = 4
	}

	if currentVersion < 5 {
		log.Println("Applying schema migration v5 (users and sessions)...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				username TEXT UNIQUE NOT NULL,
				password_hash TEXT NOT NULL,
				display_name TEXT,
				email TEXT,
				profile_image_url TEXT
			);

			CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				expires_at DATETIME NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
		`)
		if err != nil {
			return fmt.Errorf("failed to apply schema v5: %w", err)
		}
		_, err = db.Exec("PRAGMA user_version = 5;")
		if err != nil {
			return err
		}
		currentVersion = 5
	}

	if currentVersion < 6 {
		log.Println("Applying schema migration v6 (account status columns)...")
		_, err := db.Exec(`
			ALTER TABLE accounts ADD COLUMN last_sync_status TEXT DEFAULT 'idle';
			ALTER TABLE accounts ADD COLUMN last_sync_error TEXT;
		`)
		if err != nil {
			log.Printf("Warning v6: %v", err)
		}
		_, err = db.Exec("PRAGMA user_version = 6;")
		if err != nil {
			return err
		}
		currentVersion = 6
	}

	if currentVersion < 7 {
		log.Println("Applying schema migration v7 (app_settings and date fix)...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS app_settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			);

			INSERT OR IGNORE INTO app_settings (key, value) VALUES ('ignore_words', 're:,fwd:,the,and,for,this,that,with,from,your,have,status,update,alert,notification');

			UPDATE messages SET date = date * 1000 WHERE date < 100000000000;
			UPDATE messages SET internal_date = internal_date * 1000 WHERE internal_date < 100000000000;
		`)
		if err != nil {
			log.Printf("Warning v7: %v", err)
		}
		_, err = db.Exec("PRAGMA user_version = 7;")
		if err != nil {
			return err
		}
		currentVersion = 7
	}

	if currentVersion < 8 {
		log.Println("Applying schema migration v8 (agents table)...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS agents (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				schema_json TEXT NOT NULL
			);
		`)
		if err != nil {
			log.Printf("Warning v8: %v", err)
		}
		_, err = db.Exec("PRAGMA user_version = 8;")
		if err != nil {
			return err
		}
		currentVersion = 8
	}

	log.Printf("Database schema is up to date (version %d).", SchemaVersion)
	return nil
}

func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// Agent functions
func SaveAgent(a *Agent) error {
	_, err := db.Exec(`
		INSERT INTO agents (id, name, description, schema_json)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			schema_json = EXCLUDED.schema_json;
	`, a.ID, a.Name, a.Description, a.SchemaJSON)
	return err
}

func GetAgent(id string) (*Agent, error) {
	a := &Agent{}
	err := db.QueryRow("SELECT id, name, description, schema_json FROM agents WHERE id = ?", id).
		Scan(&a.ID, &a.Name, &a.Description, &a.SchemaJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func ListAgents() ([]*Agent, error) {
	rows, err := db.Query("SELECT id, name, description, schema_json FROM agents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.SchemaJSON); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func DeleteAgent(id string) error {
	_, err := db.Exec("DELETE FROM agents WHERE id = ?", id)
	return err
}

// App Settings functions
func GetSetting(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func UpdateSetting(key, value string) error {
	_, err := db.Exec(`
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = EXCLUDED.value;
	`, key, value)
	return err
}

// User functions
func SaveUser(u *User) error {
	_, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, display_name, email, profile_image_url)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username = EXCLUDED.username,
			password_hash = EXCLUDED.password_hash,
			display_name = EXCLUDED.display_name,
			email = EXCLUDED.email,
			profile_image_url = EXCLUDED.profile_image_url;
	`, u.ID, u.Username, u.PasswordHash, u.DisplayName, u.Email, u.ProfileImageURL)
	return err
}

func GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, password_hash, display_name, email, profile_image_url FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.ProfileImageURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func GetUserByID(id string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, password_hash, display_name, email, profile_image_url FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.ProfileImageURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func SaveSession(s *Session) error {
	_, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)", s.ID, s.UserID, s.ExpiresAt)
	return err
}

func GetSession(id string) (*Session, error) {
	s := &Session{}
	err := db.QueryRow("SELECT id, user_id, expires_at FROM sessions WHERE id = ?", id).
		Scan(&s.ID, &s.UserID, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func DeleteSession(id string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// Account functions
func SaveAccount(acc *account.Account) error {
	_, err := db.Exec(`
		INSERT INTO accounts (id, name, email, host, port, user, password, ssl, smtp_host, smtp_port, last_sync_status, last_sync_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			user = EXCLUDED.user,
			password = EXCLUDED.password,
			ssl = EXCLUDED.ssl,
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			last_sync_status = EXCLUDED.last_sync_status,
			last_sync_error = EXCLUDED.last_sync_error;
	`, acc.ID, acc.Name, acc.Email, acc.Host, acc.Port, acc.User, acc.Password, acc.SSL, acc.SMTPHost, acc.SMTPPort, acc.LastSyncStatus, acc.LastSyncError)
	return err
}

func GetAccount(id string) (*account.Account, error) {
	acc := &account.Account{}
	err := db.QueryRow("SELECT id, name, email, host, port, user, password, ssl, smtp_host, smtp_port, last_sync_status, last_sync_error FROM accounts WHERE id = ?", id).
		Scan(&acc.ID, &acc.Name, &acc.Email, &acc.Host, &acc.Port, &acc.User, &acc.Password, &acc.SSL, &acc.SMTPHost, &acc.SMTPPort, &acc.LastSyncStatus, &acc.LastSyncError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return acc, err
}

func ListAccounts() ([]*account.Account, error) {
	rows, err := db.Query("SELECT id, name, email, host, port, user, password, ssl, smtp_host, smtp_port, last_sync_status, last_sync_error FROM accounts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accs []*account.Account
	for rows.Next() {
		acc := &account.Account{}
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Email, &acc.Host, &acc.Port, &acc.User, &acc.Password, &acc.SSL, &acc.SMTPHost, &acc.SMTPPort, &acc.LastSyncStatus, &acc.LastSyncError); err != nil {
			return nil, err
		}
		accs = append(accs, acc)
	}
	return accs, nil
}

func DeleteAccount(id string) error {
	_, err := db.Exec("DELETE FROM accounts WHERE id = ?", id)
	return err
}

func UpdateAccountStatus(id string, status string, lastError string) error {
	_, err := db.Exec("UPDATE accounts SET last_sync_status = ?, last_sync_error = ? WHERE id = ?", status, lastError, id)
	return err
}

// Message functions
func SaveMessage(m *message.Message) error {
	to, _ := json.Marshal(m.To)
	cc, _ := json.Marshal(m.Cc)
	bcc, _ := json.Marshal(m.Bcc)
	flags, _ := json.Marshal(m.Flags)

	_, err := db.Exec(`
		INSERT OR IGNORE INTO messages (id, account_id, uid, message_id, content_hash, normalized_body, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date, body, html_body, header, flags, size, internal_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, m.ID, m.AccountID, m.UID, m.MessageID, m.ContentHash, m.NormalizedBody, m.From, string(to), string(cc), string(bcc), m.Subject, m.Date.UnixMilli(), m.Body, m.HTMLBody, m.Header, string(flags), m.Size, m.InternalDate.UnixMilli())
	return err
}

func ListMessagesFiltered(accountID string, filter AnalyticsFilter, limit, offset int) ([]*message.Message, error) {
	query := "SELECT id, account_id, uid, message_id, content_hash, normalized_body, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date, body, html_body, header, flags, size, internal_date FROM messages"
	args := []interface{}{}
	
	var clauses []string
	if accountID != "" {
		clauses = append(clauses, "account_id = ?")
		args = append(args, accountID)
	}
	if filter.Date != "" {
		clauses = append(clauses, "strftime('%Y-%m-%d', date / 1000, 'unixepoch') = ?")
		args = append(args, filter.Date)
	}
	if filter.From != "" {
		clauses = append(clauses, "from_addr = ?")
		args = append(args, filter.From)
	}
	if filter.Topic != "" {
		clauses = append(clauses, "subject LIKE ?")
		args = append(args, "%"+filter.Topic+"%")
	}

	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}

	query += " ORDER BY date DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*message.Message
	for rows.Next() {
		m := &message.Message{}
		var to, cc, bcc, flags string
		var date, internalDate int64
		err := rows.Scan(&m.ID, &m.AccountID, &m.UID, &m.MessageID, &m.ContentHash, &m.NormalizedBody, &m.From, &to, &cc, &bcc, &m.Subject, &date, &m.Body, &m.HTMLBody, &m.Header, &flags, &m.Size, &internalDate)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(to), &m.To)
		json.Unmarshal([]byte(cc), &m.Cc)
		json.Unmarshal([]byte(bcc), &m.Bcc)
		json.Unmarshal([]byte(flags), &m.Flags)
		m.Date = time.UnixMilli(date)
		m.InternalDate = time.UnixMilli(internalDate)
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func ListMessages(accountID string, limit, offset int) ([]*message.Message, error) {
	return ListMessagesFiltered(accountID, AnalyticsFilter{}, limit, offset)
}

func GetMessageByID(id string) (*message.Message, error) {
	m := &message.Message{}
	var to, cc, bcc, flags string
	var date, internalDate int64
	err := db.QueryRow("SELECT id, account_id, uid, message_id, content_hash, normalized_body, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date, body, html_body, header, flags, size, internal_date FROM messages WHERE id = ?", id).
		Scan(&m.ID, &m.AccountID, &m.UID, &m.MessageID, &m.ContentHash, &m.NormalizedBody, &m.From, &to, &cc, &bcc, &m.Subject, &date, &m.Body, &m.HTMLBody, &m.Header, &flags, &m.Size, &internalDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(to), &m.To)
	json.Unmarshal([]byte(cc), &m.Cc)
	json.Unmarshal([]byte(bcc), &m.Bcc)
	json.Unmarshal([]byte(flags), &m.Flags)
	m.Date = time.UnixMilli(date)
	m.InternalDate = time.UnixMilli(internalDate)
	return m, nil
}

func MessageExistsByMessageID(messageID string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE message_id = ?", messageID).Scan(&count)
	return count > 0, err
}

// Mailbox Sync State
func SaveMailboxSyncState(s *MailboxSyncState) error {
	_, err := db.Exec(`
		INSERT INTO mailboxes (id, account_id, name, last_uid, last_modseq)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_uid = EXCLUDED.last_uid,
			last_modseq = EXCLUDED.last_modseq;
	`, s.ID, s.AccountID, s.Name, s.LastUID, s.LastMODSEQ)
	return err
}

func GetMailboxSyncState(id string) (*MailboxSyncState, error) {
	s := &MailboxSyncState{}
	err := db.QueryRow("SELECT id, account_id, name, last_uid, last_modseq FROM mailboxes WHERE id = ?", id).
		Scan(&s.ID, &s.AccountID, &s.Name, &s.LastUID, &s.LastMODSEQ)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// Analytics Helpers
func applyFilters(query string, filter AnalyticsFilter, args []interface{}) (string, []interface{}) {
	var clauses []string
	if filter.Date != "" {
		clauses = append(clauses, "strftime('%Y-%m-%d', date / 1000, 'unixepoch') = ?")
		args = append(args, filter.Date)
	}
	if filter.From != "" {
		clauses = append(clauses, "from_addr = ?")
		args = append(args, filter.From)
	}
	if filter.Topic != "" {
		clauses = append(clauses, "subject LIKE ?")
		args = append(args, "%"+filter.Topic+"%")
	}

	if len(clauses) > 0 {
		if strings.Contains(strings.ToUpper(query), "WHERE") {
			query += " AND " + strings.Join(clauses, " AND ")
		} else {
			query += " WHERE " + strings.Join(clauses, " AND ")
		}
	}
	return query, args
}

// Analytics functions
func GetTemporalVolume(filter AnalyticsFilter) ([]AnalyticsData, error) {
	query := "SELECT strftime('%Y-%m-%d', date / 1000, 'unixepoch') as day, COUNT(*) FROM messages"
	args := []interface{}{}
	query, args = applyFilters(query, filter, args)
	
	if !strings.Contains(strings.ToUpper(query), "WHERE") {
		query += " WHERE date > 0"
	} else {
		query += " AND date > 0"
	}
	query += " GROUP BY day ORDER BY day ASC"
	
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []AnalyticsData
	for rows.Next() {
		var d AnalyticsData
		if err := rows.Scan(&d.Label, &d.Value); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, nil
}

func GetTopSenders(filter AnalyticsFilter) ([]AnalyticsData, error) {
	query := `
		SELECT from_addr, COUNT(*) as count 
		FROM messages 
		WHERE from_addr NOT IN (SELECT email FROM accounts)
		AND from_addr NOT LIKE '%david.d.fullmer@gmail.com%'
	`
	args := []interface{}{}
	query, args = applyFilters(query, filter, args)
	query += " GROUP BY from_addr ORDER BY count DESC LIMIT 10"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []AnalyticsData
	for rows.Next() {
		var d AnalyticsData
		if err := rows.Scan(&d.Label, &d.Value); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, nil
}

func GetTopicStats(filter AnalyticsFilter) ([]AnalyticsData, error) {
	ignoreStr, _ := GetSetting("ignore_words")
	ignoreWords := strings.Split(strings.ToLower(ignoreStr), ",")
	
	query := "SELECT LOWER(SUBSTR(subject, 1, INSTR(subject || ' ', ' ') - 1)) as topic, COUNT(*) as count FROM messages"
	args := []interface{}{}
	query, args = applyFilters(query, filter, args)
	
	if !strings.Contains(strings.ToUpper(query), "WHERE") {
		query += " WHERE topic != ''"
	} else {
		query += " AND topic != ''"
	}
	query += " GROUP BY topic ORDER BY count DESC LIMIT 50"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []AnalyticsData
	for rows.Next() {
		var d AnalyticsData
		if err := rows.Scan(&d.Label, &d.Value); err != nil {
			return nil, err
		}
		
		isIgnored := false
		for _, w := range ignoreWords {
			cleanW := strings.TrimSpace(w)
			if cleanW != "" && (d.Label == cleanW || len(d.Label) <= 2) {
				isIgnored = true
				break
			}
		}
		if !isIgnored {
			data = append(data, d)
		}
		if len(data) >= 10 {
			break
		}
	}
	return data, nil
}

func GetAccountStats(accountID string) (*account.AccountStats, error) {
	stats := &account.AccountStats{}
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE account_id = ?", accountID).Scan(&stats.TotalMessages)
	if err != nil {
		return nil, err
	}
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE account_id = ? AND flags NOT LIKE '%\\Seen%'", accountID).Scan(&stats.UnreadMessages)
	if err != nil {
		return nil, err
	}
	err = db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM messages WHERE account_id = ?", accountID).Scan(&stats.StorageSize)
	if err != nil {
		return nil, err
	}
	if stats.TotalMessages > 0 {
		var lastDate int64
		err = db.QueryRow("SELECT MAX(date) FROM messages WHERE account_id = ?", accountID).Scan(&lastDate)
		if err == nil {
			stats.LastSync = time.UnixMilli(lastDate).Format(time.RFC3339)
		}
	} else {
		stats.LastSync = "Never"
	}
	return stats, nil
}
