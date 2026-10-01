package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/web/connection"
	"github.com/komari-monitor/komari/web/security"
)

type WebSocketUpgradeOption func(*websocket.Upgrader)

func IsWebSocketUpgrade(c *gin.Context) bool {
	return websocket.IsWebSocketUpgrade(c.Request)
}

func EnableWebSocketCompression(upgrader *websocket.Upgrader) {
	upgrader.EnableCompression = true
}

func UpgradeWebSocket(c *gin.Context, options ...WebSocketUpgradeOption) (*websocket.Conn, error) {
	if !IsWebSocketUpgrade(c) {
		return nil, fmt.Errorf("require websocket upgrade")
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: CheckWebSocketOrigin,
	}
	for _, option := range options {
		option(&upgrader)
	}
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

// UpgradeSafeConn upgrades the request to a bounded, concurrency-safe WebSocket.
func UpgradeSafeConn(c *gin.Context, options ...WebSocketUpgradeOption) (*connection.SafeConn, error) {
	unsafeConn, err := UpgradeWebSocket(c, options...)
	if err != nil {
		return nil, err
	}
	return connection.NewSafeConn(unsafeConn), nil
}

func CheckWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	enabled, _ := config.GetAs[bool](config.WsOriginCheckEnabledKey, true)
	if !enabled {
		return true
	}
	if origin == "" {
		return false
	}
	if security.OriginMatchesHost(origin, r.Host) {
		return true
	}
	allowlist, _ := config.GetAs[string](config.WsAllowedOriginsKey, "")
	return security.OriginInAllowlist(origin, allowlist)
}
