package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Config is what the agent remembers between runs: the pairing token the
// station page holds, the port, and which portal origins may call it. It lives
// in the user's config folder (Windows: %AppData%\USCF Portal Print Agent, macOS:
// ~/Library/Application Support/USCF Portal Print Agent). An older "USCF Print
// Agent" folder is moved over, so pairing survives the rename.
type Config struct {
	Port    int      `json:"port"`
	Token   string   `json:"token"`
	Origins []string `json:"origins"`

	path string
	mu   sync.Mutex
}

const defaultPort = 17870

var defaultOrigins = []string{
	"https://portal.uscompoundingfactory.com",
	"https://api-sandbox.uscompoundingfactory.com",
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	old := filepath.Join(dir, "USCF Print Agent")
	dir = filepath.Join(dir, ProductName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, err := os.Stat(old); err == nil {
			_ = os.Rename(old, dir)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig reads the saved config, or returns defaults when there is none.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	c := &Config{Port: defaultPort, Origins: append([]string(nil), defaultOrigins...), path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if len(c.Origins) == 0 {
		c.Origins = append([]string(nil), defaultOrigins...)
	}
	return c, nil
}

// Save writes the config atomically with owner-only permissions (it holds the token).
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		return nil // in-memory config (tests)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// AllowsOrigin says whether a browser origin may talk to the agent.
func (c *Config) AllowsOrigin(origin string) bool {
	for _, o := range c.Origins {
		if o == origin {
			return true
		}
	}
	return false
}
