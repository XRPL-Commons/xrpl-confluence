// Package finding owns the durable representation of findings. M1 ships only
// the ID helpers; the store, server endpoints, and detectors land in M2.
package finding

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ulid.Monotonic's state is not safe for concurrent use; entropyMu serializes
// callers so two goroutines never advance the monotonic counter at the same time.
var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

func newID(prefix string) string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return prefix + ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// NewFindingID returns a fresh "fnd_<ULID>" identifier.
func NewFindingID() string { return newID("fnd_") }

// NewRunID returns a fresh "run_<ULID>" identifier.
func NewRunID() string { return newID("run_") }

// NewReproducerID returns a fresh "rpr_<ULID>" identifier.
func NewReproducerID() string { return newID("rpr_") }
