package store

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// NewULID returns a new ULID string. Suitable as a primary key (sortable, opaque).
func NewULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now().UTC()), rand.Reader).String()
}
