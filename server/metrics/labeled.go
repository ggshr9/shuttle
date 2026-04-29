package metrics

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

// labelTuple encodes a list of label values as a stable map key.
type labelTuple string

func makeTuple(values []string) labelTuple {
	return labelTuple(strings.Join(values, "\x00"))
}

func (t labelTuple) values() []string {
	return strings.Split(string(t), "\x00")
}

type labeledCounter struct {
	name      string
	labelKeys []string
	mu        sync.RWMutex
	counts    map[labelTuple]*atomic.Int64
}

func newLabeledCounter(name string, labelKeys []string) *labeledCounter {
	return &labeledCounter{
		name:      name,
		labelKeys: labelKeys,
		counts:    make(map[labelTuple]*atomic.Int64),
	}
}

func (c *labeledCounter) Inc(values ...string) {
	if len(values) != len(c.labelKeys) {
		panic(fmt.Sprintf("labeledCounter %s: expected %d labels, got %d", c.name, len(c.labelKeys), len(values)))
	}
	tup := makeTuple(values)

	c.mu.RLock()
	v, ok := c.counts[tup]
	c.mu.RUnlock()
	if ok {
		v.Add(1)
		return
	}

	c.mu.Lock()
	if v, ok = c.counts[tup]; !ok {
		v = &atomic.Int64{}
		c.counts[tup] = v
	}
	c.mu.Unlock()
	v.Add(1)
}

// write emits the counter in Prometheus text format.
func (c *labeledCounter) write(w io.Writer, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, help, c.name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for tup, v := range c.counts {
		labels := tup.values()
		var sb strings.Builder
		sb.WriteString(c.name)
		sb.WriteByte('{')
		for i, k := range c.labelKeys {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `%s=%q`, k, labels[i])
		}
		sb.WriteByte('}')
		fmt.Fprintf(w, "%s %d\n", sb.String(), v.Load())
	}
}
