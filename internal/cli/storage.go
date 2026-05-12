package cli

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/AFDEAPAC/kish/internal/config"
)

type storageCheckFlags struct {
	configPath string
}

func newStorageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Inspect and diagnose Kish storage backends",
	}
	cmd.AddCommand(newStorageCheckCmd())
	return cmd
}

func newStorageCheckCmd() *cobra.Command {
	var flags storageCheckFlags
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run a storage backend smoke test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStorageCheck(cmd.Context(), cmd.OutOrStdout(), flags)
		},
	}
	cmd.Flags().StringVar(&flags.configPath, "config", "kish.yaml", "Path to YAML config file")
	return cmd
}

func runStorageCheck(ctx context.Context, out interface{ Write([]byte) (int, error) }, flags storageCheckFlags) error {
	cfg, err := config.Load(flags.configPath, config.Overrides{})
	if err != nil {
		return err
	}
	storageType := cfg.Storage.Type
	if storageType == "" {
		storageType = "local"
	}

	fmt.Fprintf(out, "storage type: %s\n", storageType)
	if storageType == "s3" {
		fmt.Fprintf(out, "endpoint: %s\n", cfg.Storage.S3.Endpoint)
		fmt.Fprintf(out, "bucket: %s\n", cfg.Storage.S3.Bucket)
		fmt.Fprintf(out, "tls ca_file: %s\n", cfg.Storage.S3.TLS.CAFile)
		fmt.Fprintf(out, "tls insecure_skip_verify: %t\n", cfg.Storage.S3.TLS.InsecureSkipVerify)
		if cfg.Storage.S3.TLS.InsecureSkipVerify {
			fmt.Fprintln(out, "WARNING: storage.s3.tls.insecure_skip_verify is enabled. TLS certificate verification is disabled for S3 storage. Do not use this in production.")
		}
	}

	objStore, err := newObjectStore(ctx, cfg.Storage)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "[OK] initialized storage backend")

	key := fmt.Sprintf("diagnostics/storage-check-%d.txt", time.Now().UTC().UnixNano())
	body := []byte("kish storage check\n")
	if err := objStore.PutObject(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		return fmt.Errorf("put diagnostic object: %w", err)
	}
	fmt.Fprintln(out, "[OK] put diagnostic object")

	if err := objStore.DeleteObject(ctx, key); err != nil {
		return fmt.Errorf("delete diagnostic object: %w", err)
	}
	fmt.Fprintln(out, "[OK] delete diagnostic object")

	return nil
}
