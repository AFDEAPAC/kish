package artifact

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrObjectNotFound is returned by ObjectStore implementations when artifact
// content cannot be found for a metadata record.
var ErrObjectNotFound = errors.New("object not found")

// ErrInsufficientStorage reports that an ObjectStore backend cannot accept new
// content because local or remote storage capacity has been exhausted.
var ErrInsufficientStorage = errors.New("insufficient storage")

// ObjectInfo describes stored artifact content without requiring callers to
// open the content stream.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	UpdatedAt   time.Time
}

// ObjectStore is the artifact use case's content storage port.
//
// Implementations live in outer infrastructure packages. Keys are generated
// by the application layer and must be treated as opaque identifiers by
// storage backends. Implementations must respect ctx cancellation for every
// I/O operation and must be safe for concurrent use across requests.
type ObjectStore interface {
	// PutObject writes the bytes from r to key with the given content type
	// and total size. size is the authoritative length; implementations
	// that need an exact byte count (e.g. S3 single-part upload) must read
	// up to size and may reject mismatches. Implementations must return
	// ErrInsufficientStorage when the backend reports a capacity error so
	// the HTTP layer can map it to 507. PutObject is not atomic across
	// crashes; partially written objects may persist.
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// GetObject opens a read stream and returns the matching ObjectInfo.
	// The caller is responsible for closing the returned io.ReadCloser
	// even when the surrounding handler returns early. Implementations
	// must return ErrObjectNotFound when the key does not exist so the
	// application layer can collapse it to domain artifact.ErrNotFound.
	GetObject(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)

	// DeleteObject removes the content at key. DeleteObject is idempotent:
	// implementations must return nil when the key is already absent or
	// return ErrObjectNotFound so the application layer can swallow it
	// during cleanup. Failure to delete must not leak credentials or
	// implementation details into the error message.
	DeleteObject(ctx context.Context, key string) error
}
