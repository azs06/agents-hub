package cli

import (
	"fmt"
	"os"

	"agents-hub/internal/hub"
)

func setHubEnv(cfg hub.Config) {
	if cfg.Socket.Enabled && cfg.Socket.Path != "" {
		_ = os.Setenv("A2A_HUB_SOCKET", cfg.Socket.Path)
	} else {
		_ = os.Unsetenv("A2A_HUB_SOCKET")
	}

	if cfg.HTTP.Enabled && cfg.HTTP.Host != "" && cfg.HTTP.Port != 0 {
		_ = os.Setenv("A2A_HUB_URL", fmt.Sprintf("http://%s:%d", cfg.HTTP.Host, cfg.HTTP.Port))
	} else {
		_ = os.Unsetenv("A2A_HUB_URL")
	}
}
