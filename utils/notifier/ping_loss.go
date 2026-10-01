package notifier

import (
	"fmt"
	"strings"
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
	defaultLossAlertTemplate = "⚠️ 丢包告警\n任务：{{task}}\n服务器：{{client}}\n丢包率：{{loss_rate}}%\n统计窗口：{{window}}\n时间：{{time}}"
	defaultLossRecoveryTemplate = "✅ 丢包恢复\n任务：{{task}}\n服务器：{{client}}\n当前丢包率：{{loss_rate}}%\n统计窗口：{{window}}\n时间：{{time}}"
)

var pingLossMu sync.Mutex

// CheckPingLossNotification evaluates one newly received result. The five
// minute window is configurable per task; two consecutive decisions are
// required on both edges to suppress one-off probe failures.
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
	records, err := tasks.GetPingRecords(record.Client, int(record.TaskId), now.Add(-time.Duration(windowMinutes)*time.Minute), now.Add(time.Second))
	if err != nil || len(records) < 3 {
		return
	}
	lost := 0
	for _, item := range records {
		if item.Value < 0 {
			lost++
		}
	}
	lossRate := float64(lost) * 100 / float64(len(records))

	var state models.PingLossNotificationState
	err = db.Where("task_id = ? AND client = ?", task.Id, record.Client).First(&state).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return
	}
	if err == gorm.ErrRecordNotFound {
		state = models.PingLossNotificationState{TaskId: task.Id, Client: record.Client}
	}
	var eventName, emoji, template string
	if lossRate >= threshold {
		state.AboveCount++
		state.BelowCount = 0
		if !state.AlertActive && state.AboveCount >= 2 {
			state.AlertActive = true
			eventName, emoji, template = messageEvent.PacketLoss, "⚠️", task.LossAlertTemplate
		}
	} else {
		state.BelowCount++
		state.AboveCount = 0
		if state.AlertActive && state.BelowCount >= 2 {
			state.AlertActive = false
			eventName, emoji, template = messageEvent.PacketLossRecovered, "✅", task.LossRecoveryTemplate
		}
	}
	state.UpdatedAt = now
	if err := db.Save(&state).Error; err != nil {
		return
	}
	if eventName == "" {
		return
	}
	if strings.TrimSpace(template) == "" {
		if eventName == messageEvent.PacketLoss {
			template = defaultLossAlertTemplate
		} else {
			template = defaultLossRecoveryTemplate
		}
	}
	client, err := clients.GetClientByUUID(record.Client)
	if err != nil {
		return
	}
	if err := messageSender.SendNotification(models.EventMessage{
		Event: eventName, Emoji: emoji, Time: now, Template: template,
		Task: task.Name, LossRate: fmt.Sprintf("%.2f", lossRate),
		Window: fmt.Sprintf("%d 分钟", windowMinutes), Message: task.Name,
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
