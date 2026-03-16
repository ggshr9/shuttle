package fault

import (
	"github.com/shuttleX/shuttle/transport"
)

// faultStream wraps a transport.Stream and applies fault injection rules.
type faultStream struct {
	inner    transport.Stream
	injector *Injector
}

var _ transport.Stream = (*faultStream)(nil)

func (s *faultStream) Read(b []byte) (int, error) {
	n, err := s.inner.Read(b)
	if err != nil {
		return n, err
	}
	out, faultErr, matched := s.injector.applyRules("read", s.injector.readRules, b[:n])
	if !matched {
		return n, nil
	}
	if faultErr != nil {
		return 0, faultErr
	}
	if out == nil {
		return n, nil
	}
	copy(b, out)
	return len(out), nil
}

func (s *faultStream) Write(b []byte) (int, error) {
	out, faultErr, matched := s.injector.applyRules("write", s.injector.writeRules, b)
	if !matched {
		return s.inner.Write(b)
	}
	if faultErr != nil {
		return 0, faultErr
	}
	if out == nil {
		return len(b), nil
	}
	return s.inner.Write(out)
}

func (s *faultStream) Close() error      { return s.inner.Close() }
func (s *faultStream) StreamID() uint64  { return s.inner.StreamID() }
