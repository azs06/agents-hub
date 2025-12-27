package cli

import (
	"flag"
	"fmt"
	"os"

	"agents-hub/internal/hub"
	"agents-hub/internal/tui"
	"agents-hub/internal/utils"
)

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	httpPort := fs.Int("http-port", 8080, "http port")
	noHTTP := fs.Bool("no-http", false, "disable http")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	noSocket := fs.Bool("no-socket", false, "disable unix socket")
	verbose := fs.Bool("verbose", false, "debug logging")
	orchestratorAgents := fs.String("orchestrator-agents", "", "comma-separated agent IDs for orchestrator")
	orchestratorRouter := fs.String("orchestrator-router", "", "agent ID for LLM orchestrator routing")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg := hub.DefaultConfig()
	cfg.Socket.Path = *socketPath
	cfg.Socket.Enabled = !*noSocket
	cfg.HTTP.Port = *httpPort
	cfg.HTTP.Enabled = !*noHTTP
	cfg.Orchestrator.Agents = resolveOrchestratorAgents(*orchestratorAgents)
	cfg.Orchestrator.RouterAgent = resolveOrchestratorRouter(*orchestratorRouter)
	if *verbose {
		cfg.Logging.Level = "debug"
	}

	logger := utils.NewLogger(cfg.Logging.Level)
	setHubEnv(cfg)
	if err := tui.Run(cfg, logger); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}
