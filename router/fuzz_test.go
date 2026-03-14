package router

import (
	"encoding/json"
	"testing"
)

// FuzzDoHResponseParse tests the DoH JSON response parser with random inputs.
func FuzzDoHResponseParse(f *testing.F) {
	f.Add([]byte(`{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"}]}`))
	f.Add([]byte(`{"Status":0,"Answer":[{"type":28,"data":"::1"}]}`))
	f.Add([]byte(`{"Status":0,"Answer":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var resp dohResponse
		// Should not panic regardless of input
		json.Unmarshal(data, &resp)
	})
}

// FuzzDomainTrieInsertLookup fuzzes the domain trie with random domains.
func FuzzDomainTrieInsertLookup(f *testing.F) {
	f.Add("example.com")
	f.Add("*.google.com")
	f.Add("sub.domain.example.co.uk")
	f.Add("")
	f.Add(".")
	f.Add("....")
	f.Add("a")

	f.Fuzz(func(t *testing.T, domain string) {
		trie := NewDomainTrie()
		// Should not panic
		trie.Insert(domain, "proxy")
		trie.Lookup(domain)
	})
}
