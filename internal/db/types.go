package db

// Config holds PostgreSQL connection settings.
type Config struct {
	Servers               []string `json:"db_servers"`
	Port                  int      `json:"db_port"`
	DBName                string   `json:"db_name"`
	User                  string   `json:"db_user"`
	SSLMode               string   `json:"ssl_mode"`
	SSLCert               string   `json:"-"`
	SSLKey                string   `json:"-"`
	SSLRootCert           string   `json:"-"`
	TargetSessionAttrs    string   `json:"target_session_attrs"`
	MaxConns              int32    `json:"max_conns"`
	ConnectTimeoutSeconds int      `json:"connect_timeout_seconds"`
}
