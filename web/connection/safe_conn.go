package connection

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const maxWebSocketMessageSize int64 = 1 << 20

// SafeConn wraps a WebSocket with a write mutex and a bounded read size.
// It deliberately contains no plugin or remote-control interception hooks.
type SafeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
	ID   int64
}

func NewSafeConn(conn *websocket.Conn) *SafeConn {
	conn.SetReadLimit(maxWebSocketMessageSize)
	return &SafeConn{conn: conn, ID: time.Now().UnixNano()}
}

func (sc *SafeConn) SetReadLimit(limit int64) { sc.conn.SetReadLimit(limit) }

func (sc *SafeConn) WriteMessage(messageType int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.WriteMessage(messageType, data)
}

func (sc *SafeConn) WriteJSON(v interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	return sc.WriteMessage(websocket.TextMessage, buf.Bytes())
}

func (sc *SafeConn) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.Close()
}

func (sc *SafeConn) ReadMessage() (int, []byte, error) { return sc.conn.ReadMessage() }

func (sc *SafeConn) ReadJSON(v interface{}) error { return sc.conn.ReadJSON(v) }

func (sc *SafeConn) SetReadDeadline(t time.Time) error { return sc.conn.SetReadDeadline(t) }

func (sc *SafeConn) GetConn() *websocket.Conn {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn
}

func (sc *SafeConn) SetCloseHandler(h func(code int, text string) error) {
	sc.conn.SetCloseHandler(h)
}
