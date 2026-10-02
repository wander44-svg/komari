package client

import (
	"context"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/internal/metricstore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
	v1 "github.com/komari-monitor/komari/protocol/v1"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
	"github.com/komari-monitor/komari/utils/notifier"
)

// ingest.go
// agent 上报数据的传输无关处理逻辑。所有 agent 上报均通过 v2 JSON-RPC
// 入口解析后在这里落库并更新运行时状态。

// ingestReport 保存一次负载上报并刷新运行时状态。
func ingestReport(uuid string, report v1.Report, markPresence bool) error {
	report.UUID = uuid
	report.UpdatedAt = time.Now().UTC()
	if err := clients.ReportVerify(report); err != nil {
		return err
	}
	savedReport, err := metricstore.WriteReport(context.Background(), report)
	if err != nil {
		return err
	}
	agent_runtime.RecordReport(savedReport)
	if markPresence {
		refreshPostPresence(uuid)
	}
	return nil
}

// ingestBasicInfo 保存客户端基础信息。fallbackIP 在上报未携带 IP 时用作兜底。
func ingestBasicInfo(uuid string, info map[string]interface{}, fallbackIP string) error {
	if info == nil {
		info = map[string]interface{}{}
	}
	return saveClientBasicInfo(info, uuid, fallbackIP)
}

// ingestPingResult 保存一条 ping 探测结果。
func ingestPingResult(uuid string, taskID uint, value int) error {
	record := models.PingRecord{
		Client: uuid,
		TaskId: taskID,
		Value:  value,
		Time:   time.Now().UTC(),
	}
	if err := tasks.SavePingRecord(record); err != nil {
		return err
	}
	go func() {
		// The metric batcher may flush asynchronously; evaluate after the sample
		// has had a chance to become queryable.
		time.Sleep(5 * time.Second)
		notifier.CheckPingLossNotification(record)
	}()
	return nil
}
