package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AFDEAPAC/kish/internal/config"
)

func TestLoad_DefaultsWithNoFile(t *testing.T) {
	cfg, err := config.Load("", config.Overrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port == 0 {
		t.Error("expected default server.port to be set")
	}
	if cfg.MongoDB.URI == "" {
		t.Error("expected default mongodb.uri to be set")
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kish.yaml")
	yamlContent := `
server:
  host: "127.0.0.1"
  port: 9999
mongodb:
  uri: "mongodb://testhost:27017"
  database: "testdb"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath, config.Overrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected server.host=127.0.0.1, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected server.port=9999, got %d", cfg.Server.Port)
	}
	if cfg.MongoDB.URI != "mongodb://testhost:27017" {
		t.Errorf("unexpected mongodb.uri: %q", cfg.MongoDB.URI)
	}
	if cfg.MongoDB.Database != "testdb" {
		t.Errorf("unexpected mongodb.database: %q", cfg.MongoDB.Database)
	}
}

func TestLoad_S3StorageConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kish.yaml")
	yamlContent := `
storage:
  type: "s3"
  s3:
    bucket: "kish-artifacts"
    region: "ap-southeast-1"
    endpoint: "http://127.0.0.1:9000"
    prefix: "kish/dev/artifacts"
    force_path_style: true
    access_key_id: "minio"
    secret_access_key: "secret"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath, config.Overrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Storage.Type != "s3" {
		t.Fatalf("expected storage.type=s3, got %q", cfg.Storage.Type)
	}
	if cfg.Storage.S3.Bucket != "kish-artifacts" {
		t.Errorf("unexpected s3 bucket: %q", cfg.Storage.S3.Bucket)
	}
	if cfg.Storage.S3.Region != "ap-southeast-1" {
		t.Errorf("unexpected s3 region: %q", cfg.Storage.S3.Region)
	}
	if cfg.Storage.S3.Endpoint != "http://127.0.0.1:9000" {
		t.Errorf("unexpected s3 endpoint: %q", cfg.Storage.S3.Endpoint)
	}
	if cfg.Storage.S3.Prefix != "kish/dev/artifacts" {
		t.Errorf("unexpected s3 prefix: %q", cfg.Storage.S3.Prefix)
	}
	if !cfg.Storage.S3.ForcePathStyle {
		t.Error("expected force_path_style=true")
	}
}

func TestLoad_CLIOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kish.yaml")
	yamlContent := `
server:
  port: 9999
mongodb:
  uri: "mongodb://file:27017"
  database: "filedb"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath, config.Overrides{
		Port:          8888,
		MongoURI:      "mongodb://override:27017",
		MongoDatabase: "overridedb",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8888 {
		t.Errorf("expected CLI override port=8888, got %d", cfg.Server.Port)
	}
	if cfg.MongoDB.URI != "mongodb://override:27017" {
		t.Errorf("expected CLI override uri, got %q", cfg.MongoDB.URI)
	}
	if cfg.MongoDB.Database != "overridedb" {
		t.Errorf("expected CLI override database, got %q", cfg.MongoDB.Database)
	}
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	_, err := config.Load("/nonexistent/kish.yaml", config.Overrides{})
	if err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}

func TestValidateForAPI_MissingFields(t *testing.T) {
	cfg := config.Config{} // all zero values
	if err := config.ValidateForAPI(cfg); err == nil {
		t.Error("expected validation error for empty config, got nil")
	}
}

func TestValidateForAPI_ValidConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := config.ValidateForAPI(cfg); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}
