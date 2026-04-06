package api

import (
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"

	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/speedtest"
)

func handleEventWS(w http.ResponseWriter, r *http.Request, eng *engine.Engine, filter engine.EventType) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow connections from any origin (local app)
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Type != filter {
				continue
			}
			data, _ := json.Marshal(ev)
			if err := c.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

func handleSpeedtestWS(w http.ResponseWriter, r *http.Request, eng *engine.Engine) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	cfg := eng.Config()
	ctx := r.Context()

	// Collect all servers to test
	var servers []speedtest.Server
	if cfg.Server.Addr != "" {
		servers = append(servers, speedtest.Server{
			Addr:     cfg.Server.Addr,
			Name:     cfg.Server.Name,
			Password: cfg.Server.Password,
			SNI:      cfg.Server.SNI,
		})
	}
	for _, srv := range cfg.Servers {
		servers = append(servers, speedtest.Server{
			Addr:     srv.Addr,
			Name:     srv.Name,
			Password: srv.Password,
			SNI:      srv.SNI,
		})
	}

	if len(servers) == 0 {
		data, _ := json.Marshal(map[string]string{"error": "no servers configured"})
		_ = c.Write(ctx, websocket.MessageText, data)
		return
	}

	// Send total count first
	data, _ := json.Marshal(map[string]int{"total": len(servers)})
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}

	// Stream results as they complete
	resultCh := make(chan speedtest.TestResult, len(servers))
	tester := speedtest.NewTester(nil)

	go tester.TestAllStream(ctx, servers, resultCh)

	for result := range resultCh {
		data, _ := json.Marshal(map[string]any{"result": result})
		if err := c.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}

	// Send completion message
	data, _ = json.Marshal(map[string]bool{"done": true})
	_ = c.Write(ctx, websocket.MessageText, data)
}
