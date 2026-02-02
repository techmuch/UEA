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
	"time" // Added for Message struct

	_ "github.com/mattn/go-sqlite3"
	"github.com/user/uea/internal/account"
	"github.com/user/uea/internal/message" // Added for Message struct
)

const (
	// DBNAME is the default name for the SQLite database file.
	DBNAME = "uea.db"
	// SchemaVersion is the current version of the database schema.
	SchemaVersion = 2 // Updated schema version
)

var (
	db     *sql.DB
	dbOnce sync.Once
)

// MailboxSyncState represents the synchronization state for a specific mailbox.
type MailboxSyncState struct {
	ID        string `json:"id"`        // Unique ID for the mailbox state (e.g., accountID-mailboxName)
	AccountID string `json:"accountId"` // ID of the associated account
	Name      string `json:"name"`      // Name of the mailbox (e.g., INBOX, Sent)
	LastUID   uint32 `json:"lastUid"`   // Last UID fetched from this mailbox
	LastMODSEQ uint64 `json:"lastModseq"` // Last MODSEQ from this mailbox (0 if not supported by server)
}

// InitDB initializes the SQLite database connection and sets up the schema.
func InitDB(dataDir string) (*sql.DB, error) {
	var err error
	dbOnce.Do(func() {
		dbPath := filepath.Join(dataDir, DBNAME)
		log.Printf("Initializing database at: %s", dbPath)

		// Create data directory if it doesn't exist
		if err = os.MkdirAll(dataDir, 0755); err != nil {
			err = fmt.Errorf("failed to create data directory: %w", err)
			return
		}

		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			err = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// Configure SQLite for WAL and NORMAL synchronous mode
		// These settings are per-connection, but applied to the first connection will set them globally.
		// For a more robust solution, these should be applied to each new connection.
		// For simplicity, we apply them once here.
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
				password TEXT NOT NULL, -- Will be encrypted later
				ssl BOOLEAN NOT NULL
			);

			CREATE TABLE IF NOT EXISTS mailboxes (
				id TEXT PRIMARY KEY,
				account_id TEXT NOT NULL,
				name TEXT NOT NULL,
				last_uid INTEGER DEFAULT 0,
				last_modseq INTEGER DEFAULT 0, -- IMAP MODSEQ, 0 if not supported
				FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
				UNIQUE(account_id, name)
			);

			CREATE INDEX IF NOT EXISTS idx_mailboxes_account_id ON mailboxes(account_id);
		`)
		if err != nil {
			return fmt.Errorf("failed to apply schema v1: %w", err)
		}
		_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d;", 1))
		if err != nil {
			return fmt.Errorf("failed to set schema version to 1: %w", err)
		}
		log.Println("Schema migration v1 applied.")
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
				to_addrs TEXT NOT NULL, -- Stored as comma-separated string or JSON array
				cc_addrs TEXT NOT NULL, -- Stored as comma-separated string or JSON array
				bcc_addrs TEXT NOT NULL, -- Stored as comma-separated string or JSON array
				subject TEXT NOT NULL,
				date INTEGER NOT NULL, -- Unix timestamp
				body TEXT NOT NULL,
				html_body TEXT NOT NULL,
				header BLOB NOT NULL,
				flags TEXT NOT NULL, -- Stored as comma-separated string or JSON array
				size INTEGER NOT NULL,
				internal_date INTEGER NOT NULL, -- Unix timestamp
				FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
			);

			CREATE INDEX IF NOT EXISTS idx_messages_account_id ON messages(account_id);
			CREATE INDEX IF NOT EXISTS idx_messages_message_id ON messages(message_id);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_content_hash ON messages(content_hash);
		`)
		if err != nil {
			return fmt.Errorf("failed to apply schema v2: %w", err)
		}
		_, err = db.Exec(fmt.Sprintf("PRAGMA user_version = %d;", 2))
		if err != nil {
			return fmt.Errorf("failed to set schema version to 2: %w", err)
		}
		log.Println("Schema migration v2 applied.")
	}

	log.Printf("Database schema is up to date (version %d).", SchemaVersion)
	return nil
}

// CloseDB closes the database connection.
func CloseDB() {
	if db != nil {
		db.Close()
		log.Println("Database connection closed.")
	}
}

// SaveAccount inserts a new account or updates an existing one.
func SaveAccount(acc *account.Account) error {
	_, err := db.Exec(`
		INSERT INTO accounts (id, host, port, user, password, ssl)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			user = EXCLUDED.user,
			password = EXCLUDED.password,
			ssl = EXCLUDED.ssl;
	`, acc.ID, acc.Host, acc.Port, acc.User, acc.Password, acc.SSL)
	if err != nil {
		return fmt.Errorf("failed to save account: %w", err)
	}
	return nil
}

// GetAccount retrieves an account by its ID.
func GetAccount(id string) (*account.Account, error) {
	acc := &account.Account{}
	row := db.QueryRow("SELECT id, host, port, user, password, ssl FROM accounts WHERE id = ?", id)
	err := row.Scan(&acc.ID, &acc.Host, &acc.Port, &acc.User, &acc.Password, &acc.SSL)
	if err == sql.ErrNoRows {
		return nil, nil // Account not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	return acc, nil
}

// ListAccounts retrieves all accounts.
func ListAccounts() ([]*account.Account, error) {
	rows, err := db.Query("SELECT id, host, port, user, password, ssl FROM accounts")
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*account.Account
	for rows.Next() {
		acc := &account.Account{}
		if err := rows.Scan(&acc.ID, &acc.Host, &acc.Port, &acc.User, &acc.Password, &acc.SSL); err != nil {
			return nil, fmt.Errorf("failed to scan account row: %w", err)
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// DeleteAccount deletes an account by its ID.
func DeleteAccount(id string) error {
	_, err := db.Exec("DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}
	return nil
}

// SaveMailboxSyncState saves or updates the sync state of a mailbox.
func SaveMailboxSyncState(state *MailboxSyncState) error {
	_, err := db.Exec(`
		INSERT INTO mailboxes (id, account_id, name, last_uid, last_modseq)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_uid = EXCLUDED.last_uid,
			last_modseq = EXCLUDED.last_modseq;
	`, state.ID, state.AccountID, state.Name, state.LastUID, state.LastMODSEQ)
	if err != nil {
		return fmt.Errorf("failed to save mailbox sync state: %w", err)
	}
	return nil
}

// GetMailboxSyncState retrieves the sync state of a mailbox.
func GetMailboxSyncState(id string) (*MailboxSyncState, error) {
	state := &MailboxSyncState{}
	row := db.QueryRow("SELECT id, account_id, name, last_uid, last_modseq FROM mailboxes WHERE id = ?", id)
	err := row.Scan(&state.ID, &state.AccountID, &state.Name, &state.LastUID, &state.LastMODSEQ)
	if err == sql.ErrNoRows {
		return nil, nil // Mailbox state not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mailbox sync state: %w", err)
	}
	return state, nil
}

// ListMailboxSyncStates retrieves all mailbox sync states for a given account.
func ListMailboxSyncStates(accountID string) ([]*MailboxSyncState, error) {
	rows, err := db.Query("SELECT id, account_id, name, last_uid, last_modseq FROM mailboxes WHERE account_id = ?", accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list mailbox sync states: %w", err)
	}
	defer rows.Close()

	var states []*MailboxSyncState
	for rows.Next() {
		state := &MailboxSyncState{}
		if err := rows.Scan(&state.ID, &state.AccountID, &state.Name, &state.LastUID, &state.LastMODSEQ); err != nil {
			return nil, fmt.Errorf("failed to scan mailbox sync state row: %w", err)
		}
		states = append(states, state)
	}
	return states, nil
}

// DeleteMailboxSyncState deletes a mailbox sync state by its ID.
func DeleteMailboxSyncState(id string) error {
	_, err := db.Exec("DELETE FROM mailboxes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete mailbox sync state: %w", err)
	}
	return nil
}

// SaveMessage inserts a new message or updates an existing one.
func SaveMessage(msg *message.Message) error {
	toAddrs, err := json.Marshal(msg.To)
	if err != nil {
		return fmt.Errorf("failed to marshal To addresses: %w", err)
	}
	ccAddrs, err := json.Marshal(msg.Cc)
	if err != nil {
		return fmt.Errorf("failed to marshal Cc addresses: %w", err)
	}
	bccAddrs, err := json.Marshal(msg.Bcc)
	if err != nil {
		return fmt.Errorf("failed to marshal Bcc addresses: %w", err)
	}
	flags, err := json.Marshal(msg.Flags)
	if err != nil {
		return fmt.Errorf("failed to marshal flags: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO messages (
			id, account_id, uid, message_id, content_hash, normalized_body,
			from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date,
			body, html_body, header, flags, size, internal_date
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		) ON CONFLICT(id) DO UPDATE SET
			account_id = EXCLUDED.account_id,
			uid = EXCLUDED.uid,
			message_id = EXCLUDED.message_id,
			content_hash = EXCLUDED.content_hash,
			normalized_body = EXCLUDED.normalized_body,
			from_addr = EXCLUDED.from_addr,
			to_addrs = EXCLUDED.to_addrs,
			cc_addrs = EXCLUDED.cc_addrs,
			bcc_addrs = EXCLUDED.bcc_addrs,
			subject = EXCLUDED.subject,
			date = EXCLUDED.date,
			body = EXCLUDED.body,
			html_body = EXCLUDED.html_body,
			header = EXCLUDED.header,
			flags = EXCLUDED.flags,
			size = EXCLUDED.size,
			internal_date = EXCLUDED.internal_date;
	`,
		msg.ID, msg.AccountID, msg.UID, msg.MessageID, msg.ContentHash, msg.NormalizedBody,
		msg.From, toAddrs, ccAddrs, bccAddrs, msg.Subject, msg.Date.Unix(),
		msg.Body, msg.HTMLBody, msg.Header, flags, msg.Size, msg.InternalDate.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}
	return nil
}

// GetMessage retrieves a message by its ID.
func GetMessage(id string) (*message.Message, error) {
	msg := &message.Message{}
	var toAddrs, ccAddrs, bccAddrs, flags []byte
	var dateUnix, internalDateUnix int64

	row := db.QueryRow(`
		SELECT id, account_id, uid, message_id, content_hash, normalized_body,
		       from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date,
		       body, html_body, header, flags, size, internal_date
		FROM messages WHERE id = ?
	`, id)

	err := row.Scan(
		&msg.ID, &msg.AccountID, &msg.UID, &msg.MessageID, &msg.ContentHash, &msg.NormalizedBody,
		&msg.From, &toAddrs, &ccAddrs, &bccAddrs, &msg.Subject, &dateUnix,
		&msg.Body, &msg.HTMLBody, &msg.Header, &flags, &msg.Size, &internalDateUnix,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Message not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	msg.Date = time.Unix(dateUnix, 0)
	msg.InternalDate = time.Unix(internalDateUnix, 0)

	if err := json.Unmarshal(toAddrs, &msg.To); err != nil {
		return nil, fmt.Errorf("failed to unmarshal To addresses: %w", err)
	}
	if err := json.Unmarshal(ccAddrs, &msg.Cc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Cc addresses: %w", err)
	}
	if err := json.Unmarshal(bccAddrs, &msg.Bcc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bcc addresses: %w", err)
	}
	if err := json.Unmarshal(flags, &msg.Flags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flags: %w", err)
	}

	return msg, nil
}

// GetMessageByContentHash retrieves a message by its content hash.
func GetMessageByContentHash(hash string) (*message.Message, error) {
	msg := &message.Message{}
	var toAddrs, ccAddrs, bccAddrs, flags []byte
	var dateUnix, internalDateUnix int64

	row := db.QueryRow(`
		SELECT id, account_id, uid, message_id, content_hash, normalized_body,
		       from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date,
		       body, html_body, header, flags, size, internal_date
		FROM messages WHERE content_hash = ?
	`, hash)

	err := row.Scan(
		&msg.ID, &msg.AccountID, &msg.UID, &msg.MessageID, &msg.ContentHash, &msg.NormalizedBody,
		&msg.From, &toAddrs, &ccAddrs, &bccAddrs, &msg.Subject, &dateUnix,
		&msg.Body, &msg.HTMLBody, &msg.Header, &flags, &msg.Size, &internalDateUnix,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Message not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message by content hash: %w", err)
	}

	msg.Date = time.Unix(dateUnix, 0)
	msg.InternalDate = time.Unix(internalDateUnix, 0)

	if err := json.Unmarshal(toAddrs, &msg.To); err != nil {
		return nil, fmt.Errorf("failed to unmarshal To addresses: %w", err)
	}
	if err := json.Unmarshal(ccAddrs, &msg.Cc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Cc addresses: %w", err)
	}
	if err := json.Unmarshal(bccAddrs, &msg.Bcc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bcc addresses: %w", err)
	}
	if err := json.Unmarshal(flags, &msg.Flags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flags: %w", err)
	}

	return msg, nil
}

// MessageExistsByMessageID checks if a message with a given Message-ID already exists.
func MessageExistsByMessageID(messageID string) (bool, error) {
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM messages WHERE message_id = ?)", messageID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if message by Message-ID exists: %w", err)
	}
	return exists, nil
}

// MessageExistsByContentHash checks if a message with a given content hash already exists.
func MessageExistsByContentHash(contentHash string) (bool, error) {
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM messages WHERE content_hash = ?)", contentHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if message by content hash exists: %w", err)
	}
	return exists, nil
}

// ListMessages retrieves all messages for a given account.
func ListMessages(accountID string) ([]*message.Message, error) {
	rows, err := db.Query(`
		SELECT id, account_id, uid, message_id, content_hash, normalized_body,
		       from_addr, to_addrs, cc_addrs, bcc_addrs, subject, date,
		       body, html_body, header, flags, size, internal_date
		FROM messages WHERE account_id = ?
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}
	defer rows.Close()

	var msgs []*message.Message
	for rows.Next() {
		msg := &message.Message{}
		var toAddrs, ccAddrs, bccAddrs, flags []byte
		var dateUnix, internalDateUnix int64

		if err := rows.Scan(
			&msg.ID, &msg.AccountID, &msg.UID, &msg.MessageID, &msg.ContentHash, &msg.NormalizedBody,
			&msg.From, &toAddrs, &ccAddrs, &bccAddrs, &msg.Subject, &dateUnix,
			&msg.Body, &msg.HTMLBody, &msg.Header, &flags, &msg.Size, &internalDateUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}

		msg.Date = time.Unix(dateUnix, 0)
		msg.InternalDate = time.Unix(internalDateUnix, 0)

		if err := json.Unmarshal(toAddrs, &msg.To); err != nil {
			return nil, fmt.Errorf("failed to unmarshal To addresses: %w", err)
		}
		if err := json.Unmarshal(ccAddrs, &msg.Cc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Cc addresses: %w", err)
		}
		if err := json.Unmarshal(bccAddrs, &msg.Bcc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Bcc addresses: %w", err)
		}
		if err := json.Unmarshal(flags, &msg.Flags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal flags: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}