// Package config loads and validates the agent's YAML configuration.
package config

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	PanelURL      string   `yaml:"panel_url"`
	APIKey        string   `yaml:"api_key"`
	Interface     string   `yaml:"interface"`
	IgnoreSources []string `yaml:"ignore_sources"`

	Detector Detector `yaml:"detector"`
	Limits   Limits   `yaml:"limits"`
	Servers  []Server `yaml:"servers"`
}

type Detector struct {
	Packets int           `yaml:"packets"`
	Window  time.Duration `yaml:"window"`
	Holdoff time.Duration `yaml:"holdoff"`
}

type Limits struct {
	Cooldown        time.Duration `yaml:"cooldown"`
	MaxWakesPerHour int           `yaml:"max_wakes_per_hour"`
}

type Server struct {
	Name  string   `yaml:"name"`
	ID    string   `yaml:"id"` // Pelican server identifier (the short id in the console URL)
	Ports []uint16 `yaml:"ports"`
	Match Match    `yaml:"match"`
}

// Match optionally narrows which packets count, for games that are known to be noisy.
type Match struct {
	Prefix string `yaml:"prefix"` // hex, e.g. "090800"
	Length int    `yaml:"length"`
}

// APIKeyEnv overrides api_key so the secret can stay out of the file.
const APIKeyEnv = "LAZY_AGENT_API_KEY"

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	var c Config

	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true) // a typo in a key should be an error, not silently ignored

	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if v := os.Getenv(APIKeyEnv); v != "" {
		c.APIKey = v
	}

	c.applyDefaults()

	return &c, c.Validate()
}

func (c *Config) applyDefaults() {
	c.PanelURL = strings.TrimRight(c.PanelURL, "/")

	if c.Interface == "" {
		c.Interface = "any"
	}

	if c.Detector.Packets == 0 {
		c.Detector.Packets = 3
	}

	if c.Detector.Window == 0 {
		c.Detector.Window = 5 * time.Second
	}

	if c.Detector.Holdoff == 0 {
		c.Detector.Holdoff = 30 * time.Second
	}

	if c.Limits.Cooldown == 0 {
		c.Limits.Cooldown = 5 * time.Minute
	}

	if c.Limits.MaxWakesPerHour == 0 {
		c.Limits.MaxWakesPerHour = 6
	}
}

func (c *Config) Validate() error {
	if c.PanelURL == "" || !strings.HasPrefix(c.PanelURL, "http") {
		return fmt.Errorf("panel_url must be an http(s) URL")
	}

	if c.APIKey == "" {
		return fmt.Errorf("no API key: set api_key or the %s environment variable", APIKeyEnv)
	}

	if len(c.Servers) == 0 {
		return fmt.Errorf("no servers configured")
	}

	seenName, seenPort := map[string]bool{}, map[uint16]string{}

	for i, s := range c.Servers {
		if s.Name == "" || s.ID == "" {
			return fmt.Errorf("servers[%d]: name and id are required", i)
		}

		if seenName[s.Name] {
			return fmt.Errorf("servers[%d]: duplicate name %q", i, s.Name)
		}
		seenName[s.Name] = true

		if len(s.Ports) == 0 {
			return fmt.Errorf("server %q: ports is required", s.Name)
		}

		for _, p := range s.Ports {
			if p == 0 {
				return fmt.Errorf("server %q: invalid port 0", s.Name)
			}

			if other, dup := seenPort[p]; dup {
				return fmt.Errorf("server %q: port %d is already used by %q", s.Name, p, other)
			}
			seenPort[p] = s.Name
		}

		if _, err := hex.DecodeString(s.Match.Prefix); err != nil {
			return fmt.Errorf("server %q: match.prefix must be hex: %w", s.Name, err)
		}

		if s.Match.Length < 0 {
			return fmt.Errorf("server %q: match.length cannot be negative", s.Name)
		}
	}

	if c.Detector.Packets < 1 || c.Detector.Window <= 0 || c.Detector.Holdoff < 0 {
		return fmt.Errorf("detector: packets must be >= 1, window > 0, holdoff >= 0")
	}

	if c.Limits.Cooldown < 0 || c.Limits.MaxWakesPerHour < 1 {
		return fmt.Errorf("limits: cooldown must be >= 0 and max_wakes_per_hour >= 1")
	}

	if _, err := c.IgnorePrefixes(); err != nil {
		return err
	}

	return nil
}

// IgnorePrefixes parses ignore_sources; bare addresses are treated as /32 (or /128).
func (c *Config) IgnorePrefixes() ([]netip.Prefix, error) {
	var out []netip.Prefix

	for _, s := range c.IgnoreSources {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)

			continue
		}

		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("ignore_sources: %q is not an address or CIDR", s)
		}

		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}

	return out, nil
}

// Redacted is safe to log.
func (c *Config) Redacted() string {
	key := "(unset)"
	if c.APIKey != "" {
		key = "(set)"
	}

	return fmt.Sprintf("panel=%s key=%s interface=%s servers=%d", c.PanelURL, key, c.Interface, len(c.Servers))
}
