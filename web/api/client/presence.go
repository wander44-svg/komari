package client

import (
	"sync"
	"time"

	logger "github.com/komari-monitor/komari/utils/log"
	"github.com/komari-monitor/komari/utils/notifier"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
)

const (
	readWait        = 11 * time.Second
	postPresenceTTL = 35 * time.Second
)

type postPresenceEntry struct {
	connID     int64
	timer      *time.Timer
	generation uint64
}

var postPresenceState = struct {
	sync.Mutex
	entries map[string]*postPresenceEntry
}{entries: make(map[string]*postPresenceEntry)}

func refreshPostPresence(uuid string) {
	postPresenceState.Lock()
	defer postPresenceState.Unlock()

	if entry, exists := postPresenceState.entries[uuid]; exists {
		entry.generation++
		entry.timer.Stop()
		generation := entry.generation
		entry.timer = time.AfterFunc(postPresenceTTL, func() {
			postPresenceExpired(uuid, entry.connID, generation)
		})
		agent_runtime.KeepAlivePresence(uuid, entry.connID, postPresenceTTL)
		return
	}

	connID := time.Now().UnixNano()
	agent_runtime.KeepAlivePresence(uuid, connID, postPresenceTTL)
	go notifier.OnlineNotification(uuid, connID)
	entry := &postPresenceEntry{connID: connID}
	entry.timer = time.AfterFunc(postPresenceTTL, func() {
		postPresenceExpired(uuid, connID, 0)
	})
	postPresenceState.entries[uuid] = entry
}

func postPresenceExpired(uuid string, connID int64, generation uint64) {
	postPresenceState.Lock()
	entry, ok := postPresenceState.entries[uuid]
	if !ok || entry.connID != connID || entry.generation != generation {
		postPresenceState.Unlock()
		return
	}
	delete(postPresenceState.entries, uuid)
	postPresenceState.Unlock()

	agent_runtime.SetPresence(uuid, connID, false)
	notifier.OfflineNotification(uuid, connID)
	logger.Debugf("client-api", "POST presence expired for client %s", uuid)
}
