//go:build !linux

package relay

import "io"

// trySplice is a no-op on non-Linux platforms.
func trySplice(a, b io.ReadWriteCloser) (int64, int64, bool) {
	return 0, 0, false
}
