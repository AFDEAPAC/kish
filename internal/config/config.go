// Package config defines the runtime configuration structure for the kish binary.
//
// Config is shared between the api and upload commands. Each command uses only
// the fields relevant to it; unused fields are ignored.
package config

// Config is the top-level runtime configuration for the kish binary.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MongoDB MongoDBConfig `mapstructure:"mongodb"`
	Limits  LimitsConfig  `mapstructure:"limits"`
	Storage StorageConfig `mapstructure:"storage"`
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
	}
}
