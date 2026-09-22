package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/utils"
)

// Bootstrap initializes the data directory, primary database, and settings.
func (a *App) Bootstrap() error {
	if err := os.MkdirAll("./data/theme", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create theme directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin-data", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin storage directory: %w", err)
	}

	dbcore.SetVersionID(utils.CurrentVersion + "-" + utils.VersionHash)
	if err := dbcore.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.dbReady = true
	a.addCleanup("database", func(context.Context) error { return dbcore.Close() })

	gin.SetMode(gin.ReleaseMode)
	settings, err := config.GetManyAs[config.Settings]()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}
	a.settings = settings
	if settings.PanelListenPort == 0 {
		// Preserve the port supplied by the installer/--listen flag for existing
		// installations, then make it an explicit panel setting for later edits.
		if _, portText, splitErr := net.SplitHostPort(a.listenAddr); splitErr == nil {
			if parsedPort, parseErr := strconv.Atoi(portText); parseErr == nil {
				settings.PanelListenPort = parsedPort
			}
		}
		if settings.PanelListenPort == 0 {
			settings.PanelListenPort = 25774
		}
		if err := config.Set(config.PanelListenPortKey, settings.PanelListenPort); err != nil {
			return fmt.Errorf("failed to persist panel listen port: %w", err)
		}
	}
	if settings.PanelListenPort < 1 || settings.PanelListenPort > 65535 {
		return fmt.Errorf("panel listen port must be between 1 and 65535")
	}
	host := "0.0.0.0"
	if existingHost, _, splitErr := net.SplitHostPort(a.listenAddr); splitErr == nil && existingHost != "" {
		host = existingHost
	} else if strings.Contains(a.listenAddr, ":") && !strings.Contains(a.listenAddr, "]") {
		// Keep a simple host value supplied through --listen when it has no port.
		host = strings.TrimSpace(a.listenAddr)
	}
	a.listenAddr = net.JoinHostPort(host, strconv.Itoa(settings.PanelListenPort))
	a.tlsCertFile = strings.TrimSpace(settings.PanelTLSCertFile)
	a.tlsKeyFile = strings.TrimSpace(settings.PanelTLSKeyFile)
	if (a.tlsCertFile == "") != (a.tlsKeyFile == "") {
		return fmt.Errorf("panel TLS certificate and key paths must be configured together")
	}
	if a.tlsCertFile != "" {
		for _, path := range []string{a.tlsCertFile, a.tlsKeyFile} {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return fmt.Errorf("panel TLS file %q is not accessible: %w", path, statErr)
			}
			if info.IsDir() {
				return fmt.Errorf("panel TLS path %q is a directory", path)
			}
		}
	}
	return nil
}
