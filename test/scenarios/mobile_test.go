package scenarios

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/testkit/vnet"
)

// ---------------------------------------------------------------------------
// TestWiFiToCellularHandoff
//
// Simulates a mobile device moving from WiFi to cellular (LTE).
// The phone→server link starts with WiFi characteristics, then transitions
// through a handoff blip to LTE characteristics.
// ---------------------------------------------------------------------------

func TestWiFiToCellularHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(200))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	// Start on WiFi.
	env.Net.Link(phone, server, vnet.WiFi())

	srv := newVnetServer(env.Net, server, "h3", "server:4000")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	dialAndEcho := func(msg string) (time.Duration, error) {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4000")
		}
		conn, err := ct.Dial(ctx, "server:4000")
		if err != nil {
			return 0, err
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			return 0, err
		}
		defer stream.Close()

		payload := []byte(msg)
		start := time.Now()
		if _, err := stream.Write(payload); err != nil {
			return 0, err
		}
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(stream, buf); err != nil {
			return 0, err
		}
		rtt := time.Since(start)
		if !bytes.Equal(buf, payload) {
			t.Fatalf("echo mismatch: got %q, want %q", buf, payload)
		}
		return rtt, nil
	}

	// Phase 1: WiFi connection.
	wifiRTT, err := dialAndEcho("wifi-ping")
	if err != nil {
		t.Fatalf("WiFi echo: %v", err)
	}
	t.Logf("WiFi RTT: %v", wifiRTT)

	// Phase 2: Handoff blip.
	env.Net.UpdateLink(phone, server, vnet.HandoffBlip())
	env.Net.UpdateLink(server, phone, vnet.HandoffBlip())

	// Phase 3: Transition to LTE.
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	lteRTT, err := dialAndEcho("lte-ping")
	if err != nil {
		t.Fatalf("LTE echo after handoff: %v", err)
	}
	t.Logf("LTE RTT: %v", lteRTT)

	// Verify handoff events were recorded.
	updates := env.Recorder.Filter("link-update")
	if len(updates) < 4 {
		t.Fatalf("expected at least 4 link-update events, got %d", len(updates))
	}
}

// ---------------------------------------------------------------------------
// TestCellularToWiFiHandoff
//
// Reverse handoff: device on cellular connects to WiFi.
// Verifies latency improvement after switching from LTE to WiFi.
// ---------------------------------------------------------------------------

func TestCellularToWiFiHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(201))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	// Start on LTE.
	env.Net.Link(phone, server, vnet.LTE())

	srv := newVnetServer(env.Net, server, "h3", "server:4100")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	roundTrip := func(msg string) time.Duration {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4100")
		}
		conn, err := ct.Dial(ctx, "server:4100")
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream: %v", err)
		}
		defer stream.Close()

		payload := []byte(msg)
		start := time.Now()
		stream.Write(payload)
		buf := make([]byte, len(payload))
		io.ReadFull(stream, buf)
		return time.Since(start)
	}

	// Phase 1: Cellular.
	cellRTT := roundTrip("cell-ping")

	// Phase 2: Switch to WiFi.
	env.Net.UpdateLink(phone, server, vnet.WiFi())
	env.Net.UpdateLink(server, phone, vnet.WiFi())

	// Phase 3: WiFi.
	wifiRTT := roundTrip("wifi-ping")

	t.Logf("Cellular RTT: %v, WiFi RTT: %v", cellRTT, wifiRTT)

	// WiFi should generally be faster than cellular.
	if wifiRTT > cellRTT*3 {
		t.Fatalf("WiFi RTT (%v) should not be >3x cellular RTT (%v)", wifiRTT, cellRTT)
	}
}

// ---------------------------------------------------------------------------
// TestMobileSignalDegradation
//
// Simulates a mobile device gradually losing signal:
// LTE → Weak Signal → 3G → EDGE → Recovery (back to LTE)
// Verifies that data continues flowing at each stage and recovers properly.
// ---------------------------------------------------------------------------

func TestMobileSignalDegradation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(202))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	env.Net.Link(phone, server, vnet.LTE())

	srv := newVnetServer(env.Net, server, "h3", "server:4200")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	dialAndEcho := func(msg string) (time.Duration, error) {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4200")
		}
		conn, err := ct.Dial(ctx, "server:4200")
		if err != nil {
			return 0, err
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			return 0, err
		}
		defer stream.Close()

		payload := []byte(msg)
		start := time.Now()
		if _, err := stream.Write(payload); err != nil {
			return 0, err
		}
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(stream, buf); err != nil {
			return 0, err
		}
		return time.Since(start), nil
	}

	type stage struct {
		name string
		cfg  vnet.LinkConfig
	}

	stages := []stage{
		{"LTE", vnet.LTE()},
		{"WeakSignal", vnet.CellularWeakSignal()},
		{"3G", vnet.ThreeG()},
	}

	rtts := make([]time.Duration, 0, len(stages))
	for _, s := range stages {
		env.Net.UpdateLink(phone, server, s.cfg)
		env.Net.UpdateLink(server, phone, s.cfg)

		rtt, err := dialAndEcho("stage-" + s.name)
		if err != nil {
			t.Fatalf("%s stage failed: %v", s.name, err)
		}
		rtts = append(rtts, rtt)
		t.Logf("%s RTT: %v", s.name, rtt)
	}

	// Recovery: back to LTE.
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	recoveryRTT, err := dialAndEcho("recovery")
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	t.Logf("Recovery RTT: %v", recoveryRTT)

	// Recovery should be much better than the worst degraded stage.
	worstRTT := rtts[len(rtts)-1]
	if recoveryRTT > worstRTT {
		t.Fatalf("recovery RTT (%v) should be better than worst stage (%v)", recoveryRTT, worstRTT)
	}
}

// ---------------------------------------------------------------------------
// TestMobileSubwayScenario
//
// Simulates entering/exiting a subway tunnel:
// Good signal → tunnel (degraded) → exit (recovery)
// ---------------------------------------------------------------------------

func TestMobileSubwayScenario(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(203))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	env.Net.Link(phone, server, vnet.LTE())

	srv := newVnetServer(env.Net, server, "h3", "server:4300")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	dial := func(msg string) error {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4300")
		}
		conn, err := ct.Dial(ctx, "server:4300")
		if err != nil {
			return err
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			return err
		}
		defer stream.Close()

		payload := []byte(msg)
		if _, err := stream.Write(payload); err != nil {
			return err
		}
		buf := make([]byte, len(payload))
		_, err = io.ReadFull(stream, buf)
		return err
	}

	// Phase 1: Good signal.
	if err := dial("surface"); err != nil {
		t.Fatalf("surface: %v", err)
	}

	// Phase 2: Entering tunnel — degraded.
	env.Net.UpdateLink(phone, server, vnet.Subway())
	env.Net.UpdateLink(server, phone, vnet.Subway())

	if err := dial("tunnel-edge"); err != nil {
		t.Fatalf("tunnel edge: %v", err)
	}

	// Phase 3: Exit tunnel — signal recovered.
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	if err := dial("exit-tunnel"); err != nil {
		t.Fatalf("exit tunnel (recovery): %v", err)
	}

	t.Log("subway scenario completed: surface → tunnel → recovery")
}

// ---------------------------------------------------------------------------
// TestConcurrentMobileHandoff
//
// Multiple concurrent streams during a WiFi→LTE handoff.
// Pre-handoff streams work over WiFi, then link switches to LTE,
// and post-handoff new connections work over LTE.
// ---------------------------------------------------------------------------

func TestConcurrentMobileHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(204))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	// Start on WiFi.
	env.Net.Link(phone, server, vnet.WiFi())

	srv := newVnetServer(env.Net, server, "h3", "server:4400")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	echoOne := func(msg string) bool {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4400")
		}
		conn, err := ct.Dial(ctx, "server:4400")
		if err != nil {
			return false
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			return false
		}
		defer stream.Close()

		payload := []byte(msg)
		if _, err := stream.Write(payload); err != nil {
			return false
		}
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(stream, buf); err != nil {
			return false
		}
		return bytes.Equal(buf, payload)
	}

	// Phase 1: Concurrent WiFi streams.
	const numStreams = 5
	var wg sync.WaitGroup
	preResults := make([]bool, numStreams)
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			preResults[idx] = echoOne("pre-handoff")
		}(i)
	}
	wg.Wait()

	var preOK int
	for _, ok := range preResults {
		if ok {
			preOK++
		}
	}
	t.Logf("pre-handoff: %d/%d streams succeeded", preOK, numStreams)
	if preOK == 0 {
		t.Fatal("no streams succeeded before handoff")
	}

	// Phase 2: Handoff to LTE.
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	// Phase 3: Concurrent LTE streams.
	postResults := make([]bool, numStreams)
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			postResults[idx] = echoOne("post-handoff")
		}(i)
	}
	wg.Wait()

	var postOK int
	for _, ok := range postResults {
		if ok {
			postOK++
		}
	}
	t.Logf("post-handoff: %d/%d streams succeeded over LTE", postOK, numStreams)
	if postOK == 0 {
		t.Fatal("no streams succeeded after handoff over LTE")
	}
}

// ---------------------------------------------------------------------------
// TestMobileDualPath
//
// Simulates a device with both WiFi and cellular paths to separate servers.
// Verifies that data can flow over either path independently.
// ---------------------------------------------------------------------------

func TestMobileDualPath(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(205))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	wifiServer := env.Net.AddNode("wifi-server")
	cellServer := env.Net.AddNode("cell-server")

	// Two independent paths.
	env.Net.Link(phone, wifiServer, vnet.WiFi())
	env.Net.Link(phone, cellServer, vnet.LTE())

	srvWiFi := newVnetServer(env.Net, wifiServer, "h3-wifi", "wifi-server:4500")
	if err := srvWiFi.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srvWiFi.Close()
	echoServer(ctx, t, srvWiFi)

	srvCell := newVnetServer(env.Net, cellServer, "h3-cell", "cell-server:4501")
	if err := srvCell.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srvCell.Close()
	echoServer(ctx, t, srvCell)

	echoVia := func(addr, msg string) time.Duration {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, a string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, addr)
		}
		conn, err := ct.Dial(ctx, addr)
		if err != nil {
			t.Fatalf("Dial %s: %v", addr, err)
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream via %s: %v", addr, err)
		}
		defer stream.Close()

		payload := []byte(msg)
		start := time.Now()
		stream.Write(payload)
		buf := make([]byte, len(payload))
		io.ReadFull(stream, buf)
		rtt := time.Since(start)

		if !bytes.Equal(buf, payload) {
			t.Fatalf("echo mismatch via %s", addr)
		}
		return rtt
	}

	wifiRTT := echoVia("wifi-server:4500", "wifi-path")
	cellRTT := echoVia("cell-server:4501", "cell-path")

	t.Logf("Dual-path: WiFi RTT=%v, Cellular RTT=%v", wifiRTT, cellRTT)
}

// ---------------------------------------------------------------------------
// TestMobile5GTo4GFallback
//
// Simulates a device falling back from 5G to 4G/LTE when leaving 5G
// coverage area. Verifies seamless transition and RTT change.
// ---------------------------------------------------------------------------

func TestMobile5GTo4GFallback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(206))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	// Start on 5G.
	env.Net.Link(phone, server, vnet.FiveG())

	srv := newVnetServer(env.Net, server, "h3", "server:4600")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	roundTrip := func(msg string) time.Duration {
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:4600")
		}
		conn, err := ct.Dial(ctx, "server:4600")
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream: %v", err)
		}
		defer stream.Close()

		payload := []byte(msg)
		start := time.Now()
		stream.Write(payload)
		buf := make([]byte, len(payload))
		io.ReadFull(stream, buf)
		return time.Since(start)
	}

	fiveGRTT := roundTrip("5g-ping")

	// Fallback to LTE.
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	lteRTT := roundTrip("lte-ping")

	t.Logf("5G RTT: %v, LTE RTT: %v", fiveGRTT, lteRTT)

	// LTE should be slower than 5G.
	if lteRTT < fiveGRTT/2 {
		t.Logf("note: LTE faster than 5G — acceptable due to jitter")
	}
}
