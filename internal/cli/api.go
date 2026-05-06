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
	appAuth "github.com/AFDEAPAC/kish/internal/application/auth"
	appBootstrap "github.com/AFDEAPAC/kish/internal/application/bootstrap"
	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/config"
	"github.com/AFDEAPAC/kish/internal/infrastructure/mongodb"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage/local"
	infrahttp "github.com/AFDEAPAC/kish/internal/interfaces/http"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
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
  POST   /api/auth/login
  POST   /api/auth/refresh
  POST   /api/auth/logout
  GET    /api/auth/me
  POST   /api/users               (admin)
  GET    /api/users               (admin)
  GET    /api/users/{user_id}     (admin)
  PATCH  /api/users/{user_id}     (admin)
  DELETE /api/users/{user_id}     (admin)
  GET    /api/me
  PATCH  /api/me
  POST   /api/me/password
  POST   /api/me/client-tokens
  GET    /api/me/client-tokens
  DELETE /api/me/client-tokens/{token_id}
  POST   /api/v1/testcases        (authenticated)
  GET    /api/testcases/{id}      (backward compat, public)
  GET    /api/v1/testcases/{case_id}/artifacts
  PUT    /api/v1/testcases/{case_id}/artifacts/{artifact_name}  (authenticated)
  GET    /api/v1/testcases/{case_id}/artifacts/{artifact_name}
  DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}  (authenticated)

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

// runAPI loads config, connects to MongoDB, initialises all services, runs the
// bootstrap flow, and starts the HTTP server.
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

	// Initialise object store.
	log.Printf("[api] storage backend: %s", cfg.Storage.Type)
	if cfg.Storage.Type != "local" && cfg.Storage.Type != "" {
		return fmt.Errorf("unsupported storage.type %q; only \"local\" is supported", cfg.Storage.Type)
	}
	objStore, err := local.New(cfg.Storage.Local.Root)
	if err != nil {
		return fmt.Errorf("local storage: %w", err)
	}
	log.Printf("[api] local storage root: %s", objStore.Root())

	// Connect MongoDB.
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

	// Initialise repositories.
	tcRepo, err := mongodb.NewTestCaseRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("testcase repository init: %w", err)
	}
	artRepo, err := mongodb.NewArtifactRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("artifact repository init: %w", err)
	}
	userRepo, err := mongodb.NewUserRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("user repository init: %w", err)
	}
	sessionRepo, err := mongodb.NewSessionRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("session repository init: %w", err)
	}
	ctRepo, err := mongodb.NewClientTokenRepository(ctx, mongoClient.DB())
	if err != nil {
		return fmt.Errorf("client token repository init: %w", err)
	}

	// Initialise security infrastructure.
	hasher := security.NewBcryptHasher()
	jwtSvc := security.NewJWTService(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL)

	// Initialise application services.
	tcSvc := apptestcase.NewService(tcRepo)
	artSvc := appArtifact.NewService(tcRepo, artRepo, objStore)
	userSvc := appUser.NewService(userRepo, hasher, cfg.Auth.PasswordMinLength)
	authSvc := appAuth.NewService(userRepo, sessionRepo, hasher, jwtSvc, cfg.Auth.RefreshTokenTTL)
	ctSvc := appClientToken.NewService(ctRepo, cfg.ClientToken.Prefix)
	bootstrapSvc := appBootstrap.NewService(userRepo, hasher, cfg.Bootstrap, cfg.Auth.PasswordMinLength)

	// Bootstrap: create initial admin if none exists.
	bootstrapSvc.Run(ctx)

	// Initialise auth middleware.
	authMW := middleware.Auth(jwtSvc, ctSvc, cfg.ClientToken.Prefix)

	// Register routes.
	mux := http.NewServeMux()
	infrahttp.RegisterRoutes(
		mux,
		handler.NewHealthHandler(),
		handler.NewTestCaseHandler(tcSvc),
		handler.NewArtifactHandler(artSvc),
		handler.NewAuthHandler(authSvc, userSvc),
		handler.NewUserHandler(userSvc),
		handler.NewMeHandler(userSvc),
		handler.NewClientTokenHandler(ctSvc),
		authMW,
	)

	// Wrap the entire mux with the auth middleware so the principal is available
	// on every request context before routing.
	wrappedMux := infrahttp.WrapWithAuth(mux, authMW)

	srv := infrahttp.NewServer(cfg.Server.Host, cfg.Server.Port, wrappedMux)

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
