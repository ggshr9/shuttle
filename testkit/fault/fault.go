// Package fault provides declarative fault injection for net.Conn and
// transport.Stream. It replaces ad-hoc error mocks with composable,
// reusable fault rules that can inject delays, errors, data corruption,
// and silent drops.
//
// Usage:
//
//	fi := fault.New()
//	fi.OnRead().Delay(100*time.Millisecond).WithProbability(0.3).Install()
//	fi.OnWrite().Error(io.ErrClosedPipe).After(5*time.Second).Times(1).Install()
//	wrapped := fi.WrapConn(realConn)
package fault

import (
	"fmt"
	"net"
	"sync"

	"github.com/shuttle-proxy/shuttle/testkit/observe"
	"github.com/shuttle-proxy/shuttle/transport"
)

// Injector holds fault injection rules and wraps connections/streams.
type Injector struct {
	readRules  []Rule
	writeRules []Rule
	dialRules  []Rule
	mu         sync.Mutex
	recorder   *observe.Recorder
}

// New creates a new fault Injector with no rules.
func New() *Injector {
	return &Injector{}
}

// WithRecorder attaches a Recorder so that fault activations are logged.
func (fi *Injector) WithRecorder(r *observe.Recorder) *Injector {
	fi.recorder = r
	return fi
}

// OnRead returns a RuleBuilder targeting read operations.
func (fi *Injector) OnRead() *RuleBuilder {
	return &RuleBuilder{
		injector: fi,
		target:   "read",
		rule:     Rule{probability: 1.0},
	}
}

// OnWrite returns a RuleBuilder targeting write operations.
func (fi *Injector) OnWrite() *RuleBuilder {
	return &RuleBuilder{
		injector: fi,
		target:   "write",
		rule:     Rule{probability: 1.0},
	}
}

// OnDial returns a RuleBuilder targeting dial operations.
func (fi *Injector) OnDial() *RuleBuilder {
	return &RuleBuilder{
		injector: fi,
		target:   "dial",
		rule:     Rule{probability: 1.0},
	}
}

// WrapConn wraps a net.Conn so that reads and writes pass through fault rules.
func (fi *Injector) WrapConn(c net.Conn) net.Conn {
	return &faultConn{inner: c, injector: fi}
}

// WrapStream wraps a transport.Stream so that reads and writes pass through fault rules.
func (fi *Injector) WrapStream(s transport.Stream) transport.Stream {
	return &faultStream{inner: s, injector: fi}
}

// applyRules evaluates rules in order and applies the first matching rule.
// Returns the (possibly modified) data and error. If no rule matches,
// returns the original data with nil error.
func (fi *Injector) applyRules(target string, rules []Rule, data []byte) ([]byte, error, bool) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	for i := range rules {
		if rules[i].matches() {
			out, err := rules[i].action.Apply(data)
			if fi.recorder != nil {
				detail := fmt.Sprintf("%s on %s", rules[i].action.Name(), target)
				if err != nil {
					detail += fmt.Sprintf(" → error: %v", err)
				}
				fi.recorder.Record(observe.Event{
					Kind:   "fault",
					From:   "injector",
					Detail: detail,
					Size:   len(data),
				})
			}
			return out, err, true
		}
	}
	return data, nil, false
}
