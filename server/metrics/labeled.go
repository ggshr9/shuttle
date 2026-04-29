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

type histogramSeries struct {
	bucketCounts []atomic.Int64
	sum          atomic.Int64 // stored as nanos for time-like values
	total        atomic.Int64
}

type labeledHistogram struct {
	name      string
	labelKeys []string
	buckets   []float64
	mu        sync.RWMutex
	series    map[labelTuple]*histogramSeries
}

func newLabeledHistogram(name string, buckets []float64, labelKeys []string) *labeledHistogram {
	bb := make([]float64, len(buckets))
	copy(bb, buckets)
	return &labeledHistogram{
		name:      name,
		labelKeys: labelKeys,
		buckets:   bb,
		series:    make(map[labelTuple]*histogramSeries),
	}
}

func (h *labeledHistogram) Observe(value float64, labels ...string) {
	if len(labels) != len(h.labelKeys) {
		panic(fmt.Sprintf("labeledHistogram %s: expected %d labels, got %d", h.name, len(h.labelKeys), len(labels)))
	}
	tup := makeTuple(labels)

	h.mu.RLock()
	s, ok := h.series[tup]
	h.mu.RUnlock()
	if !ok {
		h.mu.Lock()
		if s, ok = h.series[tup]; !ok {
			s = &histogramSeries{bucketCounts: make([]atomic.Int64, len(h.buckets))}
			h.series[tup] = s
		}
		h.mu.Unlock()
	}

	for i, b := range h.buckets {
		if value <= b {
			s.bucketCounts[i].Add(1)
		}
	}
	s.sum.Add(int64(value * 1e9)) // store as nanos to preserve precision
	s.total.Add(1)
}

func (h *labeledHistogram) write(w io.Writer, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, help, h.name)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for tup, s := range h.series {
		labelVals := tup.values()
		labelStr := h.formatLabels(labelVals, "")
		for i, b := range h.buckets {
			extra := fmt.Sprintf(`le=%q`, formatFloat(b))
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, h.formatLabels(labelVals, extra), s.bucketCounts[i].Load())
			_ = i
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, h.formatLabels(labelVals, `le="+Inf"`), s.total.Load())
		sumSecs := float64(s.sum.Load()) / 1e9
		fmt.Fprintf(w, "%s_sum%s %s\n", h.name, labelStr, formatFloat(sumSecs))
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, labelStr, s.total.Load())
	}
}

func (h *labeledHistogram) formatLabels(vals []string, extra string) string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range h.labelKeys {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `%s=%q`, k, vals[i])
	}
	if extra != "" {
		if len(h.labelKeys) > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(extra)
	}
	sb.WriteByte('}')
	return sb.String()
}
