package admin

import (
	"testing"

	"github.com/shuttleX/shuttle/config"
)

func TestUserStoreAddAndList(t *testing.T) {
	s := NewUserStore(nil)
	u, err := s.Add("alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "alice" || u.Token == "" || !u.Enabled {
		t.Fatalf("unexpected user: %+v", u)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 user, got %d", len(list))
	}
	if list[0].Name != "alice" {
		t.Fatalf("expected alice, got %s", list[0].Name)
	}
}

func TestUserStoreAuthenticate(t *testing.T) {
	s := NewUserStore([]config.User{
		{Name: "bob", Token: "tok123", Enabled: true},
		{Name: "eve", Token: "tok456", Enabled: false},
	})

	if u := s.Authenticate("tok123"); u == nil || u.Name != "bob" {
		t.Fatal("expected bob for tok123")
	}
	if u := s.Authenticate("tok456"); u != nil {
		t.Fatal("disabled user should not authenticate")
	}
	if u := s.Authenticate("nonexistent"); u != nil {
		t.Fatal("unknown token should not authenticate")
	}
}

func TestUserStoreRemove(t *testing.T) {
	s := NewUserStore([]config.User{
		{Name: "charlie", Token: "tok789", Enabled: true},
	})
	if !s.Remove("tok789") {
		t.Fatal("expected successful removal")
	}
	if s.Remove("tok789") {
		t.Fatal("double remove should return false")
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty list after removal")
	}
}

func TestUserStoreSetEnabled(t *testing.T) {
	s := NewUserStore([]config.User{
		{Name: "dave", Token: "tokABC", Enabled: true},
	})
	if !s.SetEnabled("tokABC", false) {
		t.Fatal("expected successful disable")
	}
	if u := s.Authenticate("tokABC"); u != nil {
		t.Fatal("disabled user should not authenticate")
	}
	if !s.SetEnabled("tokABC", true) {
		t.Fatal("expected successful re-enable")
	}
	if u := s.Authenticate("tokABC"); u == nil {
		t.Fatal("re-enabled user should authenticate")
	}
}

func TestUserStoreToConfig(t *testing.T) {
	s := NewUserStore(nil)
	_, _ = s.Add("u1", 1000)
	_, _ = s.Add("u2", 0)

	cfg := s.ToConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 config users, got %d", len(cfg))
	}
}

func TestUserStoreTraffic(t *testing.T) {
	s := NewUserStore([]config.User{
		{Name: "frank", Token: "tokXYZ", Enabled: true},
	})
	u := s.Authenticate("tokXYZ")
	u.BytesSent.Add(1024)
	u.BytesRecv.Add(2048)
	u.ActiveConns.Add(1)

	list := s.List()
	if list[0].BytesSent != 1024 || list[0].BytesRecv != 2048 || list[0].ActiveConns != 1 {
		t.Fatalf("traffic not tracked: %+v", list[0])
	}
}

func TestUserStoreSetEnabledNotFound(t *testing.T) {
	s := NewUserStore(nil)
	if s.SetEnabled("nonexistent", true) {
		t.Fatal("should return false for unknown token")
	}
}

func TestGenerateToken(t *testing.T) {
	tok1, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	tok2, _ := generateToken()
	if tok1 == tok2 {
		t.Fatal("tokens should be unique")
	}
	if len(tok1) != 64 { // 32 bytes = 64 hex chars
		t.Fatalf("expected 64-char token, got %d", len(tok1))
	}
}
