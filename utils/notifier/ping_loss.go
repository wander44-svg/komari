package notifier

import (
	"strconv"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/models/messageEvent"
	"github.com/komari-monitor/komari/database/tasks"
	logger "github.com/komari-monitor/komari/utils/log"
	"github.com/komari-monitor/komari/utils/messageSender"
	"gorm.io/gorm"
)

const (
	defaultLossThreshold = 20.0
	defaultLossWindow = 5
)

var pingLossMu sync.Mutex

// CheckPingLossNotification evaluates one newly received result. The window
// is configurable per task; the persisted AlertActive state suppresses
// repeated alerts until the window falls back below the configured threshold.
func CheckPingLossNotification(record models.PingRecord) {
	if record.Client == "" || record.TaskId == 0 {
		return
	}

	pingLossMu.Lock()
	defer pingLossMu.Unlock()

	db := dbcore.GetDBInstance()
	var task models.PingTask
	if err := db.First(&task, record.TaskId).Error; err != nil || !task.LossNotifyEnabled {
		return
	}
	lossClients := task.LossClients
	if len(lossClients) == 0 {
		// Existing tasks predate this setting; default to the same servers that
		// already run the latency task until the administrator narrows the list.
		lossClients = task.Clients
	}
	if !containsString(lossClients, record.Client) {
		return
	}

	windowMinutes := task.LossWindowMinutes
	if windowMinutes <= 0 {
		windowMinutes = defaultLossWindow
	}
	threshold := task.LossThreshold
	if threshold <= 0 {
		threshold = defaultLossThreshold
	}
	if threshold > 100 {
		threshold = 100
	}
	now := record.Time
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lost, total, err := tasks.GetPingLossStats(record.Client, int(record.TaskId), now.Add(-time.Duration(windowMinutes)*time.Minute), now.Add(time.Second))
	if err != nil || total < 3 {
		return
	}
	lossRate := float64(lost) * 100 / float64(total)

	var state models.PingLossNotificationState
	err = db.Where("task_id = ? AND client = ?", task.Id, record.Client).First(&state).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return
	}
	if err == gorm.ErrRecordNotFound {
		state = models.PingLossNotificationState{TaskId: task.Id, Client: record.Client}
	}
	var eventName, emoji, thresholdText string
	if lossRate >= threshold {
		state.AboveCount++
		state.BelowCount = 0
		if !state.AlertActive {
			state.AlertActive = true
			eventName, emoji, thresholdText = messageevent.PacketLoss, "⚠️", strconv.FormatFloat(threshold, 'f', -1, 64)+"%"
		}
	} else {
		state.BelowCount++
		state.AboveCount = 0
		if state.AlertActive {
			state.AlertActive = false
			eventName, emoji = messageevent.PacketLossRecovered, "✅"
		}
	}
	state.UpdatedAt = now
	if err := db.Save(&state).Error; err != nil {
		return
	}
	if eventName == "" {
		return
	}
	client, err := clients.GetClientByUUID(record.Client)
	if err != nil {
		return
	}
	if err := messageSender.SendNotification(models.EventMessage{
		Event: eventName, Emoji: emoji, Time: now, Threshold: thresholdText, Message: task.Name,
		Clients: []models.Client{client},
	}); err != nil {
		logger.Errorf("notifier", "Failed to send ping loss notification for task %d/client %s: %v", task.Id, record.Client, err)
	}
}

func containsString(values models.StringArray, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
