package api

import (
	"encoding/json"
	"net/http"

	"nhooyr.io/websocket"

	"github.com/shuttle-proxy/shuttle/engine"
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
