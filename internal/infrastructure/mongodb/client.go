// Package mongodb provides infrastructure-layer implementations backed by MongoDB.
//
// All MongoDB-specific details (BSON mapping, connection handling, index creation)
// are contained here. The domain and application layers interact only with the
// domain/testcase.Repository interface.
package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client wraps a mongo.Client and holds the target database name.
type Client struct {
	client   *mongo.Client
	database string
}

// Connect establishes a MongoDB connection using the provided URI and returns
// a Client. The caller must call Disconnect when done.
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

// Disconnect closes the MongoDB connection.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// DB returns the named mongo.Database.
func (c *Client) DB() *mongo.Database {
	return c.client.Database(c.database)
}
