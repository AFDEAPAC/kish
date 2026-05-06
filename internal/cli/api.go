package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	appArtifact "github.com/AFDEAPAC/kish/internal/application/artifact"
	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/config"
	"github.com/AFDEAPAC/kish/internal/infrastructure/mongodb"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage/local"
	infrahttp "github.com/AFDEAPAC/kish/internal/interfaces/http"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
)

type apiFlags struct {
	configPath    string
	host          string
	port          int
	mongoURI      string
	mongoDatabase string
}

// newAPICmd constructs the `kish api` cobra command.
func newAPICmd() *cobra.Command {
	var flags apiFlags

	cmd := &cobra.Command{
		Use:   "api",
		Short: "Start the kish REST API server",
		Long: `api starts the kish REST API server backed by MongoDB.

The server exposes:
  GET    /healthz
  POST   /api/testcases
  PUT    /api/testcases/{id}
  GET    /api/testcases/{id}
  GET    /api/v1/testcases/{case_id}/artifacts
  PUT    /api/v1/testcases/{case_id}/artifacts/{artifact_name}
  GET    /api/v1/testcases/{case_id}/artifacts/{artifact_name}
  DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}

Configuration is read from a YAML file. CLI flags override file values.

Examples:
  kish api
  kish api --config kish.yaml
  kish api --port 8080 --mongo-uri mongodb://mongo:27017`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.configPath, "config", "kish.yaml", "Path to YAML config file")
	cmd.Flags().StringVar(&flags.host, "host", "", "Override server bind host")
	cmd.Flags().IntVar(&flags.port, "port", 0, "Override server port")
	cmd.Flags().StringVar(&flags.mongoURI, "mongo-uri", "", "Override MongoDB URI")
	cmd.Flags().StringVar(&flags.mongoDatabase, "mongo-database", "", "Override MongoDB database name")

	return cmd
}

// runAPI loads config, connects to MongoDB, initialises storage, wires up the
// HTTP server, and blocks until an OS signal or startup error causes shutdown.
func runAPI(ctx context.Context, flags apiFlags) error {
	cfg, err := config.Load(flags.configPath, config.Overrides{
		Host:          flags.host,
		Port:          flags.port,
		MongoURI:      flags.mongoURI,
		MongoDatabase: flags.mongoDatabase,
	})
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := config.ValidateForAPI(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Initialise object store based on configured storage type.
	// Only "local" is supported in this release; unknown types are rejected.
	log.Printf("[api] storage backend: %s", cfg.Storage.Type)
	if cfg.Storage.Type != "local" && cfg.Storage.Type != "" {
		return fmt.Errorf("unsupported storage.type %q; only \"local\" is supported", cfg.Storage.Type)
	}
	objStore, err := local.New(cfg.Storage.Local.Root)
	if err != nil {
		return fmt.Errorf("local storage: %w", err)
	}
	log.Printf("[api] local storage root: %s", objStore.Root())

	log.Printf("[api] connecting to MongoDB at %s (db: %s)", cfg.MongoDB.URI, cfg.MongoDB.Database)
	mongoClient, err := mongodb.Connect(ctx, cfg.MongoDB.URI, cfg.MongoDB.Database)
	if err != nil {
		return fmt.Errorf("mongodb: %w", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(shutCtx); err != nil {
			log.Printf("[api] mongodb disconnect: %v", err)
		}
	}()

	tcRepo, err := mongodb.NewTestCaseRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("testcase repository init: %w", err)
	}

	artRepo, err := mongodb.NewArtifactRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("artifact repository init: %w", err)
	}

	tcSvc := apptestcase.NewService(tcRepo)
	artSvc := appArtifact.NewService(tcRepo, artRepo, objStore)

	mux := http.NewServeMux()
	infrahttp.RegisterRoutes(mux,
		handler.NewHealthHandler(),
		handler.NewTestCaseHandler(tcSvc),
		handler.NewArtifactHandler(artSvc),
	)

	srv := infrahttp.NewServer(cfg.Server.Host, cfg.Server.Port, mux)

	// Listen for OS signals to trigger graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start()
	}()

	select {
	case err := <-serverErr:
		return err
	case sig := <-quit:
		log.Printf("[api] received signal %s, shutting down", sig)
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
