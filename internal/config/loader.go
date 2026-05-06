package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Overrides carries optional field-level overrides applied after loading the
// YAML file. Fields with zero values are ignored, so callers only need to set
// the fields they wish to override (typically from CLI flags).
type Overrides struct {
	Host          string
	Port          int
	MongoURI      string
	MongoDatabase string
}

// Load reads the YAML config file at path, applies overrides, and validates
// required fields for the api command.
//
// If path is empty, Viper still applies defaults and overrides; no file is read.
// The returned Config always has DefaultConfig values as the baseline.
func Load(path string, overrides Overrides) (Config, error) {
	cfg := DefaultConfig()

	v := viper.New()
	v.SetConfigType("yaml")

	// Seed Viper with defaults so that missing YAML keys fall back gracefully.
	v.SetDefault("server.host", cfg.Server.Host)
	v.SetDefault("server.port", cfg.Server.Port)
	v.SetDefault("mongodb.uri", cfg.MongoDB.URI)
	v.SetDefault("mongodb.database", cfg.MongoDB.Database)
	v.SetDefault("limits.test_result_max_bytes", cfg.Limits.TestResultMaxBytes)
	v.SetDefault("limits.test_script_max_bytes", cfg.Limits.TestScriptMaxBytes)
	v.SetDefault("limits.environment_snapshot_max_bytes", cfg.Limits.EnvironmentSnapshotMaxBytes)
	v.SetDefault("storage.type", cfg.Storage.Type)
	v.SetDefault("storage.local.root", cfg.Storage.Local.Root)
	v.SetDefault("storage.s3.region", cfg.Storage.S3.Region)
	v.SetDefault("storage.s3.force_path_style", cfg.Storage.S3.ForcePathStyle)
	v.SetDefault("auth.access_token_ttl", cfg.Auth.AccessTokenTTL)
	v.SetDefault("auth.refresh_token_ttl", cfg.Auth.RefreshTokenTTL)
	v.SetDefault("auth.password_min_length", cfg.Auth.PasswordMinLength)
	v.SetDefault("auth.client_token_encryption_key", cfg.Auth.ClientTokenEncryptionKey)
	v.SetDefault("client_token.prefix", cfg.ClientToken.Prefix)
	v.SetDefault("client_token.default_ttl", cfg.ClientToken.DefaultTTL)
	v.SetDefault("client_token.allow_unlimited", cfg.ClientToken.AllowUnlimited)
	v.SetDefault("bootstrap.enabled", cfg.Bootstrap.Enabled)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("could not read config file %q: %w", path, err)
		}
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("could not parse config: %w", err)
	}

	// CLI flag overrides take precedence over the config file.
	if overrides.Host != "" {
		cfg.Server.Host = overrides.Host
	}
	if overrides.Port != 0 {
		cfg.Server.Port = overrides.Port
	}
	if overrides.MongoURI != "" {
		cfg.MongoDB.URI = overrides.MongoURI
	}
	if overrides.MongoDatabase != "" {
		cfg.MongoDB.Database = overrides.MongoDatabase
	}

	return cfg, nil
}

// ValidateForAPI checks that all fields required by the api command are present.
func ValidateForAPI(cfg Config) error {
	var errs []error
	if cfg.MongoDB.URI == "" {
		errs = append(errs, errors.New("mongodb.uri is required"))
	}
	if cfg.MongoDB.Database == "" {
		errs = append(errs, errors.New("mongodb.database is required"))
	}
	if cfg.Server.Host == "" {
		errs = append(errs, errors.New("server.host is required"))
	}
	if cfg.Server.Port == 0 {
		errs = append(errs, errors.New("server.port is required"))
	}
	return errors.Join(errs...)
}
