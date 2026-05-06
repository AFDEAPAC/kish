// Package config defines the runtime configuration structure for the kish binary.
//
// Config is shared between the api and upload commands. Each command uses only
// the fields relevant to it; unused fields are ignored.
package config

import "time"

// Config is the top-level runtime configuration for the kish binary.
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	MongoDB     MongoDBConfig     `mapstructure:"mongodb"`
	Limits      LimitsConfig      `mapstructure:"limits"`
	Storage     StorageConfig     `mapstructure:"storage"`
	Auth        AuthConfig        `mapstructure:"auth"`
	ClientToken ClientTokenConfig `mapstructure:"client_token"`
	Bootstrap   BootstrapConfig   `mapstructure:"bootstrap"`
}

// ServerConfig holds HTTP server bind settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// MongoDBConfig holds MongoDB connection settings.
type MongoDBConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

// LimitsConfig holds payload size limits enforced by the API server.
// All values are in bytes.
type LimitsConfig struct {
	TestResultMaxBytes           int64 `mapstructure:"test_result_max_bytes"`
	TestScriptMaxBytes           int64 `mapstructure:"test_script_max_bytes"`
	EnvironmentSnapshotMaxBytes  int64 `mapstructure:"environment_snapshot_max_bytes"`
}

// StorageConfig holds artifact storage backend settings.
type StorageConfig struct {
	// Type selects the storage backend. Currently only "local" is supported.
	Type  string             `mapstructure:"type"`
	Local StorageLocalConfig `mapstructure:"local"`
}

// StorageLocalConfig holds settings for the local filesystem backend.
type StorageLocalConfig struct {
	// Root is the base directory under which all artifact objects are stored.
	Root string `mapstructure:"root"`
}

// AuthConfig holds JWT and password settings for the API server.
type AuthConfig struct {
	// JWTSecret is the HMAC-SHA256 signing key for access tokens.
	// Must be set to a strong random value in production.
	JWTSecret string `mapstructure:"jwt_secret"`

	// AccessTokenTTL controls how long a JWT access token remains valid.
	// Default: 24h. Internal deployments may use longer values.
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`

	// RefreshTokenTTL controls how long a refresh token remains valid.
	// Default: 720h (30 days).
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`

	// PasswordMinLength is the minimum accepted password length.
	// Default: 8.
	PasswordMinLength int `mapstructure:"password_min_length"`
}

// ClientTokenConfig holds settings for client (API) tokens used by kish upload.
type ClientTokenConfig struct {
	// Prefix is prepended to generated client tokens (e.g. "kish" → "kish_<hex>").
	Prefix string `mapstructure:"prefix"`

	// DefaultTTL is the default lifetime for a new client token when the request
	// does not specify expires_at and unlimited is false.
	DefaultTTL time.Duration `mapstructure:"default_ttl"`

	// AllowUnlimited permits creating tokens with no expiration.
	AllowUnlimited bool `mapstructure:"allow_unlimited"`
}

// BootstrapConfig controls automatic initial-admin creation at server startup.
type BootstrapConfig struct {
	// Enabled, when true, causes the server to create an initial admin account
	// on startup if no admin user exists yet.
	Enabled bool `mapstructure:"enabled"`

	// AdminEmail is the email for the bootstrap admin account.
	AdminEmail string `mapstructure:"admin_email"`

	// AdminPassword is the plaintext password for the bootstrap admin account.
	// It is hashed before storage and is never logged.
	AdminPassword string `mapstructure:"admin_password"`

	// AdminDisplayName is the display name for the bootstrap admin account.
	AdminDisplayName string `mapstructure:"admin_display_name"`
}

// DefaultConfig returns a Config populated with sane defaults.
// Callers should override individual fields from YAML or CLI flags after calling this.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 30051,
		},
		MongoDB: MongoDBConfig{
			URI:      "mongodb://localhost:27017",
			Database: "kish",
		},
		Limits: LimitsConfig{
			TestResultMaxBytes:          5 * 1024 * 1024,
			TestScriptMaxBytes:          1 * 1024 * 1024,
			EnvironmentSnapshotMaxBytes: 5 * 1024 * 1024,
		},
		Storage: StorageConfig{
			Type: "local",
			Local: StorageLocalConfig{
				Root: "./data/artifacts",
			},
		},
		Auth: AuthConfig{
			// JWTSecret has no default; it must be set explicitly in production.
			AccessTokenTTL:    24 * time.Hour,
			RefreshTokenTTL:   720 * time.Hour,
			PasswordMinLength: 8,
		},
		ClientToken: ClientTokenConfig{
			Prefix:         "kish",
			DefaultTTL:     2160 * time.Hour, // 90 days
			AllowUnlimited: true,
		},
		Bootstrap: BootstrapConfig{
			Enabled: false,
		},
	}
}
