package account

// Account represents an email account configured for synchronization.
type Account struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	Password       string `json:"password"` // This will be encrypted later
	SSL            bool   `json:"ssl"`
	SMTPHost       string `json:"smtpHost"`
	SMTPPort       int    `json:"smtpPort"`
	LastSyncStatus string `json:"lastSyncStatus"`
	LastSyncError  string `json:"lastSyncError"`
}

// AccountStats provides statistics for an account.
type AccountStats struct {
	TotalMessages  int    `json:"totalMessages"`
	UnreadMessages int    `json:"unreadMessages"`
	StorageSize    int64  `json:"storageSize"`
	LastSync       string `json:"lastSync"`
}
