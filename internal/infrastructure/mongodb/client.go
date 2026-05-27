// Package mongodb provides infrastructure-layer implementations backed by MongoDB.
//
// All MongoDB-specific details (BSON mapping, connection handling, index
// creation) are contained here. The domain and application layers depend on
// their repository interfaces; this package adapts those ports to collections
// for TestCases, artifacts, users, sessions, and client tokens.
//
// Transaction boundary: every repository method in this package is a single-
// document operation. Multi-document MongoDB transactions are not used and
// must not be added without first turning the deployment into a replica set
// and reviewing every domain-level invariant. Cross-collection consistency
// (for example, deleting a TestCase together with its artifact metadata and
// object storage) is the responsibility of the application layer.
//
// Index assumptions: each repository creates the indexes it needs at startup
// inside an ensure*Indexes helper. Operators that bypass startup (e.g.
// running an old binary against new code) must keep those indexes in place
// to preserve uniqueness and query performance guarantees.
package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client owns a *mongo.Client and the target database name. Client is the
// only handle to MongoDB inside kish; every repository in this package is
// constructed against the *mongo.Database returned by DB().
//
// Concurrency: the underlying mongo.Client is safe for concurrent use by
// many goroutines, so Client itself is too. Methods that issue queries
// respect the context passed by the caller.
type Client struct {
	client   *mongo.Client
	database string
}

// Connect dials MongoDB and verifies connectivity before returning. uri must
// be a fully formed mongodb:// or mongodb+srv:// URI sourced from
// mongodb.uri in config. The caller owns the lifetime: a successful Connect
// must be paired with Disconnect, typically in main during graceful
// shutdown after the HTTP server has stopped accepting new requests.
//
// Connect fails fast on misconfiguration by Ping-ing the server; if Ping
// fails the partially-initialised client is disconnected before returning
// so connection slots are not leaked.
func Connect(ctx context.Context, uri, database string) (*Client, error) {
	opts := options.Client().ApplyURI(uri)
	c, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb connect: %w", err)
	}

	// Ping to verify connectivity early so startup fails fast on misconfiguration.
	if err := c.Ping(ctx, nil); err != nil {
		_ = c.Disconnect(ctx)
		return nil, fmt.Errorf("mongodb ping: %w", err)
	}

	return &Client{client: c, database: database}, nil
}

// Disconnect closes the underlying connection pool. ctx bounds how long
// Disconnect will wait for outstanding operations; callers should use a
// short deadline (the API server uses 5s) so process shutdown cannot stall
// on a hung MongoDB server. Disconnect must be called after the HTTP
// server has stopped accepting new requests; calling it while requests are
// still in flight will surface as failed queries to those requests.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// DB returns the *mongo.Database for the configured database name. The
// returned handle is cheap to derive and safe to call from multiple
// goroutines.
func (c *Client) DB() *mongo.Database {
	return c.client.Database(c.database)
}
