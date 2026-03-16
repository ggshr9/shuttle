package test

import (
	"testing"
	"time"

	"github.com/shuttleX/shuttle/congestion"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/internal/pool"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/router"
)

func BenchmarkBBROnAck(b *testing.B) {
	bbr := congestion.NewBBR(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bbr.OnAck(1200, 50*time.Millisecond)
	}
}

func BenchmarkBrutalOnAck(b *testing.B) {
	brutal := congestion.NewBrutal(100 * 1024 * 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		brutal.OnAck(1200, 50*time.Millisecond)
	}
}

func BenchmarkEncryptDecrypt(b *testing.B) {
	key, err := crypto.DeriveKeys([]byte("bench-key"), 32)
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, 1200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ct, _ := crypto.Encrypt(key, nil, data)
		crypto.Decrypt(key, nil, ct)
	}
}

func BenchmarkStreamCipher(b *testing.B) {
	var key [32]byte
	derived, err := crypto.DeriveKeys([]byte("bench"), 32)
	if err != nil {
		b.Fatal(err)
	}
	copy(key[:], derived)
	enc, _ := crypto.NewStreamCipher(key, crypto.CipherChaChaPoly)
	dec, _ := crypto.NewStreamCipher(key, crypto.CipherChaChaPoly)
	data := make([]byte, 1200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ct := enc.Seal(data)
		dec.Open(ct)
	}
}

func BenchmarkReplayFilter(b *testing.B) {
	rf := crypto.NewReplayFilter(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rf.Check(uint64(i))
	}
}

func BenchmarkDomainTrieLookup(b *testing.B) {
	trie := router.NewDomainTrie()
	// Insert 10000 domains
	domains := []string{"google.com", "youtube.com", "facebook.com", "twitter.com", "github.com"}
	for i := 0; i < 10000; i++ {
		d := domains[i%len(domains)]
		trie.Insert(d, "proxy")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Lookup("google.com")
	}
}

func BenchmarkRouterMatch(b *testing.B) {
	rt := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "domain", Values: []string{"google.com", "youtube.com"}, Action: router.ActionProxy},
			{Type: "domain", Values: []string{"baidu.com", "qq.com"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.MatchDomain("google.com")
	}
}

func BenchmarkBufferPool(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(16 * 1024)
		pool.Put(buf)
	}
}

func BenchmarkPadding(b *testing.B) {
	p := obfs.NewPadder(1200)
	data := make([]byte, 500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		padded, _ := p.Pad(data)
		p.Unpad(padded)
	}
}
