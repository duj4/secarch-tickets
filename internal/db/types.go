package db

// Config holds PostgreSQL connection settings.
type Config struct {
	Host                  string `json:"db_server"`
	Port                  int    `json:"db_port"`
	DBName                string `json:"db_name"`
	User                  string `json:"db_user"`
	SSLMode               string `json:"ssl_mode"`
	SSLCert               string `json:"ssl_cert"`
	SSLKey                string `json:"ssl_key"`
	SSLRootCert           string `json:"ssl_root_cert"`
	MaxConns              int32  `json:"max_conns"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
}
