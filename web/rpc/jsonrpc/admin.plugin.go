package jsonrpc

import (
	"context"
	"errors"
	"strings"

	"github.com/komari-monitor/komari/internal/plugin"
	"github.com/komari-monitor/komari/pkg/rpc"
)

func adminListPlugins(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return plugin.List(), nil
}

// adminSetPluginEnabled is the single switch entry point. When a plugin's
// declared permissions differ from the approved hash, enabling it returns
// { requires_approval: true } instead of an error; the caller then shows a
// permission dialog and retries with approved=true.
func adminSetPluginEnabled(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	enabled, _ := rpc.GetParamAs[bool](req, "enabled")
	approved, _ := rpc.GetParamAs[bool](req, "approved")
	if err := plugin.SetEnabled(short, enabled, approved); err != nil {
		if errors.Is(err, plugin.ErrPermissionApprovalRequired) {
			return map[string]any{"requires_approval": true}, nil
		}
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func adminGetPluginLogs(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	return map[string]any{"logs": plugin.GetLogs(short)}, nil
}

func adminDeletePlugin(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	if err := plugin.Delete(short); err != nil {
		if errors.Is(err, plugin.ErrNotInstalled) {
			return nil, rpc.MakeError(rpc.NotFound, err.Error(), nil)
		}
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func adminGetPluginConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	info, err := plugin.Manifest(short)
	if err != nil {
		return nil, rpc.MakeError(rpc.NotFound, err.Error(), nil)
	}
	values, err := plugin.GetConfiguration(short)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"configuration": info.Configuration, "data": values}, nil
}

func adminSetPluginConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	data, _ := rpc.GetParamAs[map[string]any](req, "data")
	if err := plugin.SaveConfiguration(short, data); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	if err := plugin.Reload(short); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "plugin configuration saved but reload failed: "+err.Error(), nil)
	}
	return nil, nil
}
