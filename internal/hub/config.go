package hub

type Config struct {
	Socket struct {
		Path    string
		Enabled bool
	}
	HTTP struct {
		Enabled bool
		Host    string
		Port    int
	}
	Orchestrator struct {
		Agents []string
	}
	Logging struct {
		Level  string
		Pretty bool
	}
	DataDir string
}

func DefaultConfig() Config {
	cfg := Config{}
	cfg.Socket.Path = "/tmp/a2a-hub.sock"
	cfg.Socket.Enabled = true
	cfg.HTTP.Enabled = true
	cfg.HTTP.Host = "127.0.0.1"
	cfg.HTTP.Port = 8080
	cfg.Orchestrator.Agents = []string{"claude-code", "gemini", "codex", "vibe"}
	cfg.Logging.Level = "info"
	cfg.Logging.Pretty = false
	cfg.DataDir = ""
	return cfg
}
