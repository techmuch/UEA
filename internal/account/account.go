package account

// Account represents an email account configured for synchronization.
type Account struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"` // This will be encrypted later
	SSL      bool   `json:"ssl"`
}
