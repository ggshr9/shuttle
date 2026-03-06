package router

import (
	"strings"
	"sync"
)

// DomainTrie is a trie optimized for domain suffix matching.
// Domains are stored reversed (e.g., "com.google.www") for suffix matching.
type DomainTrie struct {
	mu   sync.RWMutex
	root *trieNode
	size int
}

type trieNode struct {
	children map[string]*trieNode
	value    string // action: "proxy", "direct", "reject"
	isEnd    bool
}

// NewDomainTrie creates a new domain trie.
func NewDomainTrie() *DomainTrie {
	return &DomainTrie{
		root: &trieNode{children: make(map[string]*trieNode)},
	}
}

// Insert adds a domain with its associated action.
// Supports exact match ("example.com") and wildcard ("+.example.com" for all subdomains).
func (t *DomainTrie) Insert(domain, action string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	parts := reverseDomain(domain)
	node := t.root
	for _, part := range parts {
		if node.children == nil {
			node.children = make(map[string]*trieNode)
		}
		child, ok := node.children[part]
		if !ok {
			child = &trieNode{children: make(map[string]*trieNode)}
			node.children[part] = child
		}
		node = child
	}
	if !node.isEnd {
		t.size++
	}
	node.isEnd = true
	node.value = action
}

// Lookup finds the action for a domain. Returns ("", false) if not found.
// Matches the most specific rule: exact > wildcard > parent domain.
func (t *DomainTrie) Lookup(domain string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	parts := reverseDomain(domain)
	node := t.root
	lastMatch := ""
	found := false

	for _, part := range parts {
		// Check wildcard match at current level
		if wild, ok := node.children["+"]; ok && wild.isEnd {
			lastMatch = wild.value
			found = true
		}

		child, ok := node.children[part]
		if !ok {
			break
		}
		node = child
		if node.isEnd {
			lastMatch = node.value
			found = true
		}
	}

	return lastMatch, found
}

// Delete removes a domain from the trie (soft delete).
// Returns true if the domain was found and deleted.
func (t *DomainTrie) Delete(domain string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	parts := reverseDomain(domain)
	node := t.root
	for _, part := range parts {
		child, ok := node.children[part]
		if !ok {
			return false
		}
		node = child
	}
	if !node.isEnd {
		return false
	}
	node.isEnd = false
	node.value = ""
	t.size--
	return true
}

// Size returns the number of entries in the trie.
func (t *DomainTrie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// reverseDomain splits and reverses a domain for trie storage.
// "www.google.com" → ["com", "google", "www"]
func reverseDomain(domain string) []string {
	// Remove leading "+." for wildcard domains
	domain = strings.TrimPrefix(domain, "+.")
	domain = strings.TrimSuffix(domain, ".")
	domain = strings.ToLower(domain)

	parts := strings.Split(domain, ".")
	// Reverse
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return parts
}
