package engine

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/ggshr9/shuttle/adapter"
)

// selectState holds the manually-chosen active outbound for the GroupSelect strategy.
type selectState struct {
	mu       sync.RWMutex
	selected adapter.Outbound
	members  map[string]adapter.Outbound // tag → outbound for fast lookup
}

// newSelectState builds a selectState from the given outbounds, pre-selecting the first.
func newSelectState(outbounds []adapter.Outbound) *selectState {
	s := &selectState{
		members: make(map[string]adapter.Outbound, len(outbounds)),
	}
	for _, ob := range outbounds {
		s.members[ob.Tag()] = ob
	}
	if len(outbounds) > 0 {
		s.selected = outbounds[0]
	}
	return s
}

// Select sets the active outbound by tag. Returns an error if the tag is not a member.
func (s *selectState) Select(tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ob, ok := s.members[tag]
	if !ok {
		return fmt.Errorf("select: outbound %q not found in group", tag)
	}
	s.selected = ob
	return nil
}

// Selected returns the tag of the currently selected outbound, or "" if none is set.
func (s *selectState) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.selected == nil {
		return ""
	}
	return s.selected.Tag()
}

// dialSelect dials using the manually selected outbound.
func (g *OutboundGroup) dialSelect(ctx context.Context, network, address string) (net.Conn, error) {
	if g.selectState == nil {
		return g.dialFailover(ctx, network, address)
	}

	g.selectState.mu.RLock()
	sel := g.selectState.selected
	g.selectState.mu.RUnlock()

	if sel == nil {
		return nil, fmt.Errorf("outbound group %q: no outbound selected", g.tag)
	}
	return sel.DialContext(ctx, network, address)
}

// SelectOutbound sets the active outbound in the group by tag.
// Returns an error if no selectState is configured or the tag is not a member.
func (g *OutboundGroup) SelectOutbound(tag string) error {
	if g.selectState == nil {
		return fmt.Errorf("outbound group %q: not a select-strategy group", g.tag)
	}
	return g.selectState.Select(tag)
}

// SelectedOutbound returns the tag of the currently selected outbound, or "" if none.
func (g *OutboundGroup) SelectedOutbound() string {
	if g.selectState == nil {
		return ""
	}
	return g.selectState.Selected()
}

// SetSelect configures this group for the select strategy with the given outbounds.
func (g *OutboundGroup) SetSelect(outbounds []adapter.Outbound) {
	g.selectState = newSelectState(outbounds)
}
