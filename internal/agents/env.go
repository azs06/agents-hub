package agents

import (
	"os"
	"os/exec"
)

func resolveExec(defaultExec string, envKeys ...string) string {
	for _, key := range envKeys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	if _, err := exec.LookPath(defaultExec); err == nil {
		return defaultExec
	}
	return defaultExec
}

func resolveExecWithFallback(defaultExec string, fallbackPaths []string, envKeys ...string) string {
	for _, key := range envKeys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	if _, err := exec.LookPath(defaultExec); err == nil {
		return defaultExec
	}
	for _, path := range fallbackPaths {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return defaultExec
}
