package test

import (
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/router"
)

// ---------------------------------------------------------------------------
// BenchmarkConnectionEstablishment – yamux session setup over net.Pipe
// ---------------------------------------------------------------------------

func BenchmarkConnectionEstablishment(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c1, c2 := net.Pipe()
		conf := yamux.DefaultConfig()
		conf.LogOutput = io.Discard

		done := make(chan struct{})
		go func() {
			s, err := yamux.Server(c2, conf)
			if err == nil {
				s.Close()
			}
			close(done)
		}()

		client, err := yamux.Client(c1, conf)
		if err != nil {
			b.Fatal(err)
		}
		client.Close()
		<-done
	}
}

// ---------------------------------------------------------------------------
// BenchmarkStreamOpenLatency – time to open a single stream on yamux
// ---------------------------------------------------------------------------

func BenchmarkStreamOpenLatency(b *testing.B) {
	client, server := yamuxPair(b)
	defer client.Close()
	defer server.Close()

	// Drain accepted streams in background.
	go func() {
		for {
			s, err := server.AcceptStream()
			if err != nil {
				return
			}
			s.Close()
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := client.OpenStream()
		if err != nil {
			b.Fatal(err)
		}
		s.Close()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkDNSTrieLookup – domain trie with 10 000 entries
// ---------------------------------------------------------------------------

func BenchmarkDNSTrieLookup(b *testing.B) {
	trie := router.NewDomainTrie()

	// Insert 10 000 unique domains across various TLDs.
	tlds := []string{"com", "org", "net", "io", "co"}
	for i := 0; i < 10000; i++ {
		domain := fmt.Sprintf("host%d.example.%s", i, tlds[i%len(tlds)])
		trie.Insert(domain, "proxy")
	}

	// Also insert some well-known domains for lookup targets.
	targets := []string{
		"google.com",
		"youtube.com",
		"github.com",
		"twitter.com",
		"facebook.com",
	}
	for _, d := range targets {
		trie.Insert(d, "proxy")
	}

	b.Run("hit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Lookup(targets[i%len(targets)])
		}
	})

	b.Run("miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Lookup("nonexistent.example.zz")
		}
	})

	b.Run("deep-subdomain", func(b *testing.B) {
		trie.Insert("example.com", "direct")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Lookup("a.b.c.d.e.f.example.com")
		}
	})
}

// ---------------------------------------------------------------------------
// BenchmarkRouterMatchWithGeoIP – router match with GeoIP rules loaded
// ---------------------------------------------------------------------------

func BenchmarkRouterMatchWithGeoIP(b *testing.B) {
	// Build a GeoIP database with representative CIDR blocks.
	geoIP := router.NewGeoIPDB()
	cnCIDRs := make([]string, 0, 1000)
	for i := 0; i < 250; i++ {
		// Generate /24 blocks in the 10.x.x.0 range as stand-ins for CN.
		cnCIDRs = append(cnCIDRs, fmt.Sprintf("10.%d.%d.0/24", i, i%256))
	}
	for i := 0; i < 250; i++ {
		cnCIDRs = append(cnCIDRs, fmt.Sprintf("172.%d.%d.0/24", 16+i%16, i%256))
	}
	for i := 0; i < 500; i++ {
		cnCIDRs = append(cnCIDRs, fmt.Sprintf("192.168.%d.0/24", i%256))
	}
	geoIP.LoadFromCIDRs("CN", cnCIDRs)

	usCIDRs := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		usCIDRs = append(usCIDRs, fmt.Sprintf("8.%d.%d.0/24", i/256, i%256))
	}
	geoIP.LoadFromCIDRs("US", usCIDRs)

	rt := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "domain", Values: []string{"google.com", "youtube.com", "github.com"}, Action: router.ActionProxy},
			{Type: "domain", Values: []string{"baidu.com", "qq.com", "taobao.com"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, geoIP, nil, nil)

	b.Run("domain-hit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.MatchDomain("google.com")
		}
	})

	b.Run("domain-miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.MatchDomain("unknown-domain.example.org")
		}
	})

	b.Run("ip-cn-match", func(b *testing.B) {
		ip := net.ParseIP("10.100.100.1")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.MatchIP(ip)
		}
	})

	b.Run("ip-us-match", func(b *testing.B) {
		ip := net.ParseIP("8.0.1.1")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.MatchIP(ip)
		}
	})

	b.Run("ip-no-match", func(b *testing.B) {
		ip := net.ParseIP("203.0.113.1")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.MatchIP(ip)
		}
	})

	b.Run("full-match", func(b *testing.B) {
		ip := net.ParseIP("10.100.100.1")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.Match("google.com", ip, "chrome", "")
		}
	})
}
