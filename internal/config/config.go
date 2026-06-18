// Package config handles the CLI's persisted configuration (API base URL and
// token) via Viper. Config lives at ~/.config/civitai/config.yaml and is
// overridable by CIVITAI_* environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	// DefaultBaseURL is the Civitai API base used when nothing is configured.
	DefaultBaseURL = "https://civitai.com"

	keyToken   = "token"
	keyBaseURL = "base_url"

	// EnvPrefix maps CIVITAI_TOKEN / CIVITAI_BASE_URL onto config keys.
	EnvPrefix = "CIVITAI"
)

// Config is the resolved CLI configuration.
type Config struct {
	v    *viper.Viper
	path string
}

// Dir returns the config directory (~/.config/civitai), honouring
// XDG_CONFIG_HOME.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "civitai"), nil
}

// Load reads config from disk + environment. A missing file is not an error.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	v.SetDefault(keyBaseURL, DefaultBaseURL)

	v.SetEnvPrefix(EnvPrefix)
	// CIVITAI_TOKEN -> token, CIVITAI_BASE_URL -> base_url
	_ = v.BindEnv(keyToken, "CIVITAI_TOKEN")
	_ = v.BindEnv(keyBaseURL, "CIVITAI_BASE_URL")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// A real parse error: surface it. (Not-found is fine.)
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		}
	}

	return &Config{v: v, path: filepath.Join(dir, "config.yaml")}, nil
}

// Token returns the configured API token (env overrides file).
func (c *Config) Token() string { return c.v.GetString(keyToken) }

// BaseURL returns the configured API base URL.
func (c *Config) BaseURL() string { return c.v.GetString(keyBaseURL) }

// Path returns the on-disk config file path.
func (c *Config) Path() string { return c.path }

// SetToken stores the API token and persists the config file.
func (c *Config) SetToken(token string) error {
	c.v.Set(keyToken, token)
	return c.save()
}

// SetBaseURL stores a custom base URL and persists the config file.
func (c *Config) SetBaseURL(url string) error {
	c.v.Set(keyBaseURL, url)
	return c.save()
}

func (c *Config) save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := c.v.WriteConfigAs(c.path); err != nil {
		return err
	}
	// The file holds a credential — keep it owner-only.
	return os.Chmod(c.path, 0o600)
}
