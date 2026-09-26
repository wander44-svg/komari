package jsonrpc

import (
	"context"
	"encoding/json"

	"github.com/komari-monitor/komari/database"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/rpc"
	"github.com/komari-monitor/komari/utils/messageSender"
	msfactory "github.com/komari-monitor/komari/utils/messageSender/factory"
)

// admin.provider.go
// Telegram notification configuration RPC2 methods.

func init() {
	reg("listNotificationChannels", adminListNotificationChannels, "List supported notification channels")
	reg("getNotificationChannelConfiguration", adminGetNotificationChannelConfiguration, "Get notification channel configuration")
	reg("setNotificationChannelConfiguration", adminSetNotificationChannelConfiguration, "Set notification channel configuration")
}

const telegramChannel = "telegram"
const defaultTelegramEndpoint = "https://api.telegram.org/bot"

func telegramConfiguration() map[string]any {
	configs := msfactory.GetSenderConfigs()
	return map[string]any{
		"name": "Telegram",
		"data": configs[telegramChannel],
	}
}

func adminListNotificationChannels(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return []map[string]any{{
		"id":            telegramChannel,
		"configuration": telegramConfiguration(),
	}}, nil
}

func adminGetNotificationChannelConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		ID string `json:"id"`
	}
	if err := req.BindParams(&params); err != nil || params.ID != telegramChannel {
		return nil, rpc.MakeError(rpc.InvalidParams, "Only Telegram notifications are supported", nil)
	}

	data := map[string]any{}
	if saved, err := database.GetMessageSenderConfigByName(telegramChannel); err == nil && saved.Addition != "" {
		if err := json.Unmarshal([]byte(saved.Addition), &data); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Invalid saved Telegram configuration", nil)
		}
	}
	if endpoint, ok := data["endpoint"].(string); !ok || endpoint == "" {
		data["endpoint"] = defaultTelegramEndpoint
	}
	return map[string]any{"configuration": telegramConfiguration(), "data": data}, nil
}

func adminSetNotificationChannelConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		ID   string         `json:"id"`
		Data map[string]any `json:"data"`
	}
	if err := req.BindParams(&params); err != nil || params.ID != telegramChannel {
		return nil, rpc.MakeError(rpc.InvalidParams, "Only Telegram notifications are supported", nil)
	}
	if params.Data == nil {
		params.Data = map[string]any{}
	}
	if endpoint, ok := params.Data["endpoint"].(string); !ok || endpoint == "" {
		params.Data["endpoint"] = defaultTelegramEndpoint
	}
	addition, err := json.Marshal(params.Data)
	if err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid Telegram configuration", nil)
	}
	provider := &models.MessageSenderProvider{Name: telegramChannel, Addition: string(addition)}
	if err := database.SaveMessageSenderConfig(provider); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to save Telegram configuration: "+err.Error(), nil)
	}
	if err := messageSender.LoadProvider(telegramChannel, provider.Addition); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to load Telegram configuration: "+err.Error(), nil)
	}
	return map[string]any{"message": "Telegram configuration saved"}, nil
}
