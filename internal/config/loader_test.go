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
	if cfg.CORS.Enabled {
		t.Error("expected cors.enabled default to be false")
	}
	if len(cfg.CORS.AllowedMethods) == 0 {
		t.Error("expected default CORS allowed methods")
	}
	if len(cfg.CORS.AllowedHeaders) == 0 {
		t.Error("expected default CORS allowed headers")
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
    tls:
      ca_file: "/etc/kish/certs/minio-ca.crt"
      insecure_skip_verify: true
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
	if cfg.Storage.S3.TLS.CAFile != "/etc/kish/certs/minio-ca.crt" {
		t.Errorf("unexpected s3 tls ca_file: %q", cfg.Storage.S3.TLS.CAFile)
	}
	if !cfg.Storage.S3.TLS.InsecureSkipVerify {
		t.Error("expected s3 tls insecure_skip_verify=true")
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

func TestLoad_CORSConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kish.yaml")
	yamlContent := `
cors:
  enabled: true
  allowed_origins:
    - "http://dashboard.example:30150"
  allowed_methods:
    - "GET"
    - "POST"
    - "OPTIONS"
  allowed_headers:
    - "Authorization"
    - "Content-Type"
  allow_credentials: true
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath, config.Overrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CORS.Enabled {
		t.Fatal("expected cors.enabled=true")
	}
	if got := cfg.CORS.AllowedOrigins; len(got) != 1 || got[0] != "http://dashboard.example:30150" {
		t.Fatalf("unexpected cors.allowed_origins: %#v", got)
	}
	if got := cfg.CORS.AllowedMethods; len(got) != 3 || got[1] != "POST" {
		t.Fatalf("unexpected cors.allowed_methods: %#v", got)
	}
	if got := cfg.CORS.AllowedHeaders; len(got) != 2 || got[0] != "Authorization" {
		t.Fatalf("unexpected cors.allowed_headers: %#v", got)
	}
	if !cfg.CORS.AllowCredentials {
		t.Fatal("expected cors.allow_credentials=true")
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
