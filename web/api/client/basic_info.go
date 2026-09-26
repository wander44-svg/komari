package client

import (
	"net"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/utils/geoip"
)

func saveClientBasicInfo(info map[string]interface{}, uuid string, fallbackIP string) error {
	info["uuid"] = uuid
	applyFallbackClientIP(info, fallbackIP)
	appendClientRegionFromGeoIP(info)
	return clients.SaveClientInfo(info)
}

func applyFallbackClientIP(info map[string]interface{}, fallbackIP string) {
	if hasClientIP(info) {
		return
	}
	ip := net.ParseIP(fallbackIP)
	if ip == nil {
		return
	}
	if ip.To4() != nil {
		info["ipv4"] = fallbackIP
	} else {
		info["ipv6"] = fallbackIP
	}
}

func hasClientIP(info map[string]interface{}) bool {
	if ipv4, ok := info["ipv4"].(string); ok && ipv4 != "" {
		return true
	}
	if ipv6, ok := info["ipv6"].(string); ok && ipv6 != "" {
		return true
	}
	return false
}

func appendClientRegionFromGeoIP(info map[string]interface{}) {
	enabled, err := config.GetAs[bool](config.GeoIpEnabledKey)
	if err != nil || !enabled {
		return
	}
	for _, key := range []string{"ipv4", "ipv6"} {
		ipText, ok := info[key].(string)
		if !ok || ipText == "" {
			continue
		}
		record, _ := geoip.GetGeoInfo(net.ParseIP(ipText))
		if record == nil {
			continue
		}
		if region := geoip.GetRegionUnicodeEmoji(record.ISOCode); region != "" {
			info["region"] = region
			return
		}
	}
}
