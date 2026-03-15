//go:build linux

package relay

import (
	"io"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const spliceMaxSize = 65536

// splicePair holds a pipe used for splice(2) transfer.
type splicePair struct {
	r, w int // pipe read and write fds
}

func newSplicePair() (*splicePair, error) {
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		return nil, err
	}
	return &splicePair{r: fds[0], w: fds[1]}, nil
}

func (p *splicePair) Close() {
	unix.Close(p.r)
	unix.Close(p.w)
}

// spliceOne transfers data from srcFD to dstFD through a pipe using splice(2).
// It performs two splice calls per chunk: src->pipe, then pipe->dst.
// Returns total bytes transferred.
func spliceOne(srcFD, dstFD int, pipe *splicePair) (int64, error) {
	var total int64
	for {
		// Move data from source fd into the pipe.
		n, err := unix.Splice(srcFD, nil, pipe.w, nil, spliceMaxSize, unix.SPLICE_F_NONBLOCK|unix.SPLICE_F_MOVE)
		if n > 0 {
			// Drain the pipe into the destination fd.
			// Must drain all n bytes; splice may return partial writes.
			remain := n
			for remain > 0 {
				written, werr := unix.Splice(pipe.r, nil, dstFD, nil, int(remain), unix.SPLICE_F_MOVE)
				if written > 0 {
					remain -= written
					total += written
				}
				if werr != nil {
					return total, werr
				}
			}
		}
		if err != nil {
			// EAGAIN means no data ready; treat as EOF in blocking context.
			if err == unix.EAGAIN {
				// Re-read; if source truly has no more data, next splice returns 0.
				continue
			}
			return total, err
		}
		if n == 0 {
			// EOF on source.
			return total, nil
		}
	}
}

// spliceRelay transfers data between two file descriptors using splice(2).
// Returns total bytes transferred in each direction.
func spliceRelay(aFD, bFD int) (aToB, bToA int64, err error) {
	pipe1, err := newSplicePair()
	if err != nil {
		return 0, 0, err
	}
	defer pipe1.Close()

	pipe2, err := newSplicePair()
	if err != nil {
		return 0, 0, err
	}
	defer pipe2.Close()

	var wg sync.WaitGroup
	var err1, err2 error

	wg.Add(2)

	// a -> b
	go func() {
		defer wg.Done()
		aToB, err1 = spliceOne(aFD, bFD, pipe1)
		// Shut down the write side of b to signal EOF.
		unix.Shutdown(bFD, unix.SHUT_WR)
	}()

	// b -> a
	go func() {
		defer wg.Done()
		bToA, err2 = spliceOne(bFD, aFD, pipe2)
		unix.Shutdown(aFD, unix.SHUT_WR)
	}()

	wg.Wait()

	if err1 != nil {
		return aToB, bToA, err1
	}
	return aToB, bToA, err2
}

// trySplice attempts a splice-based relay. Returns false if splice is not supported
// for the given connections (e.g., not raw TCP, or CountedReadWriter wrappers).
func trySplice(a, b io.ReadWriteCloser) (int64, int64, bool) {
	// CountedReadWriter must not use splice (byte counting would be bypassed).
	if _, ok := a.(*CountedReadWriter); ok {
		return 0, 0, false
	}
	if _, ok := b.(*CountedReadWriter); ok {
		return 0, 0, false
	}

	aConn, aOK := a.(syscall.Conn)
	bConn, bOK := b.(syscall.Conn)
	if !aOK || !bOK {
		return 0, 0, false
	}

	aRaw, err := aConn.SyscallConn()
	if err != nil {
		return 0, 0, false
	}
	bRaw, err := bConn.SyscallConn()
	if err != nil {
		return 0, 0, false
	}

	var aFD, bFD int
	var fdErr error

	// Extract file descriptors.
	if err := aRaw.Control(func(fd uintptr) { aFD = int(fd) }); err != nil {
		return 0, 0, false
	}
	if err := bRaw.Control(func(fd uintptr) { bFD = int(fd) }); err != nil {
		return 0, 0, false
	}

	// Try one test splice to see if it's supported on these fds.
	// Create a throwaway pipe and attempt a zero-length splice.
	testPipe, err := newSplicePair()
	if err != nil {
		return 0, 0, false
	}
	// Attempt a non-blocking splice of 0 bytes to verify support.
	_, testErr := unix.Splice(aFD, nil, testPipe.w, nil, 0, unix.SPLICE_F_NONBLOCK)
	testPipe.Close()
	if testErr != nil && testErr != unix.EAGAIN {
		return 0, 0, false
	}

	_ = fdErr

	n1, n2, err := spliceRelay(aFD, bFD)
	if err != nil {
		// If splice failed at runtime, caller should fall back.
		// But we already transferred some data, so report what we got.
		// In practice, if splice fails on the first call it returns 0,0.
		if n1 == 0 && n2 == 0 {
			return 0, 0, false
		}
	}
	return n1, n2, true
}
