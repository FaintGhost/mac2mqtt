// Package config loads and saves Mac2MQTT's settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Version is the current configuration schema version.
const Version = 1

// Config is the complete on-disk configuration of Mac2MQTT.
type Config struct {
	Version       int           `json:"version"`
	Broker        Broker        `json:"broker"`
	HomeAssistant HomeAssistant `json:"home_assistant"`
	Reporting     Reporting     `json:"reporting"`
	UI            UI            `json:"ui"`
}

// UI holds application interface settings.
type UI struct {
	// Language is empty to follow the system, else a BCP 47 tag such as
	// "en", "zh-Hans" or "zh-Hant".
	Language string `json:"language"`
}

// Broker describes the MQTT connection.
type Broker struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	ClientID           string `json:"client_id"`
}

// HomeAssistant describes the discovery topics and the device identity.
type HomeAssistant struct {
	DiscoveryPrefix string `json:"discovery_prefix"`
	BaseTopic       string `json:"base_topic"`
	DeviceName      string `json:"device_name"`
	// DisabledGroups names the entity groups not published. Listing what
	// is off rather than on publishes groups added later by default.
	DisabledGroups []string `json:"disabled_groups,omitempty"`
}

// Disabled returns the set of entity groups not published.
func (h HomeAssistant) Disabled() map[string]bool {
	out := make(map[string]bool, len(h.DisabledGroups))
	for _, group := range h.DisabledGroups {
		out[group] = true
	}
	return out
}

// Publishes reports whether the entity group is published.
func (h HomeAssistant) Publishes(group string) bool {
	return h.Disabled()[group] == false
}

// SetPublishes publishes the entity group or stops publishing it.
func (h *HomeAssistant) SetPublishes(group string, on bool) {
	groups := h.DisabledGroups[:0:0]
	for _, g := range h.DisabledGroups {
		if g != group {
			groups = append(groups, g)
		}
	}
	if on == false {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	h.DisabledGroups = groups
}

// Reporting controls how often state is published and whether it is retained.
type Reporting struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
	Retain          bool `json:"retain"`
}

// Default returns a usable configuration for this Mac.
func Default() Config {
	host := hostname()
	name := deviceName(host)
	clientID := "mac2mqtt-" + sanitizeID(name)
	return Config{
		Version: Version,
		Broker: Broker{
			Port:     1883,
			ClientID: clientID,
		},
		HomeAssistant: HomeAssistant{
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "mac2mqtt/" + sanitizeID(name),
			DeviceName:      name,
		},
		Reporting: Reporting{
			Enabled:         true,
			IntervalSeconds: 30,
			Retain:          true,
		},
	}
}

// Normalize fills defaults without discarding information the user entered.
func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = Version
	}
	host := hostname()
	name := deviceName(host)
	if c.Broker.Port == 0 {
		if c.Broker.TLS {
			c.Broker.Port = 8883
		} else {
			c.Broker.Port = 1883
		}
	}
	if strings.TrimSpace(c.Broker.ClientID) == "" {
		c.Broker.ClientID = "mac2mqtt-" + sanitizeID(name)
	}
	c.Broker.Host = strings.TrimSpace(c.Broker.Host)
	c.Broker.Username = strings.TrimSpace(c.Broker.Username)
	c.Broker.ClientID = strings.TrimSpace(c.Broker.ClientID)
	if strings.TrimSpace(c.HomeAssistant.DeviceName) == "" {
		c.HomeAssistant.DeviceName = name
	}
	c.HomeAssistant.DeviceName = strings.TrimSpace(c.HomeAssistant.DeviceName)
	if strings.TrimSpace(c.HomeAssistant.DiscoveryPrefix) == "" {
		c.HomeAssistant.DiscoveryPrefix = "homeassistant"
	}
	c.HomeAssistant.DiscoveryPrefix = strings.Trim(strings.TrimSpace(c.HomeAssistant.DiscoveryPrefix), "/")
	if strings.TrimSpace(c.HomeAssistant.BaseTopic) == "" {
		c.HomeAssistant.BaseTopic = "mac2mqtt/" + sanitizeID(c.HomeAssistant.DeviceName)
	}
	c.HomeAssistant.BaseTopic = strings.Trim(strings.TrimSpace(c.HomeAssistant.BaseTopic), "/")
	if c.Reporting.IntervalSeconds == 0 {
		c.Reporting.IntervalSeconds = 30
	}
	if c.Reporting.IntervalSeconds < 5 {
		c.Reporting.IntervalSeconds = 5
	}
	if c.Reporting.IntervalSeconds > 3600 {
		c.Reporting.IntervalSeconds = 3600
	}
}

// Validate reports whether the configuration can connect and publish.
func (c Config) Validate() error {
	var errs []error
	host := strings.TrimSpace(c.Broker.Host)
	if host == "" && c.Reporting.Enabled {
		errs = append(errs, errors.New("broker host is required while reporting is enabled"))
	} else if host != "" {
		if strings.Contains(host, "://") {
			errs = append(errs, errors.New("broker host must be a hostname or IP address without a URL scheme"))
		}
		if strings.ContainsAny(host, "/?#") || strings.ContainsAny(host, " \t\r\n") {
			errs = append(errs, errors.New("broker host must not contain a path, port, or whitespace"))
		}
		if strings.Count(host, ":") == 1 {
			errs = append(errs, errors.New("broker host must not include a port; use the port field"))
		}
	}
	if c.Broker.Port < 1 || c.Broker.Port > 65535 {
		errs = append(errs, errors.New("broker port must be between 1 and 65535"))
	}
	if strings.TrimSpace(c.Broker.ClientID) == "" {
		errs = append(errs, errors.New("MQTT client ID is required"))
	}
	if len(c.Broker.ClientID) > 128 {
		errs = append(errs, errors.New("MQTT client ID must be at most 128 characters"))
	}
	if strings.TrimSpace(c.HomeAssistant.DiscoveryPrefix) == "" {
		errs = append(errs, errors.New("Home Assistant discovery prefix is required"))
	}
	if strings.TrimSpace(c.HomeAssistant.BaseTopic) == "" {
		errs = append(errs, errors.New("MQTT base topic is required"))
	}
	for name, topic := range map[string]string{
		"discovery prefix": c.HomeAssistant.DiscoveryPrefix,
		"base topic":       c.HomeAssistant.BaseTopic,
	} {
		if strings.ContainsAny(topic, "+#") {
			errs = append(errs, fmt.Errorf("%s must not contain MQTT wildcards", name))
		}
	}
	if c.Reporting.IntervalSeconds < 5 || c.Reporting.IntervalSeconds > 3600 {
		errs = append(errs, errors.New("report interval must be between 5 and 3600 seconds"))
	}
	return errors.Join(errs...)
}

// NodeID is the stable, MQTT-safe identifier used in discovery topics.
func (c Config) NodeID() string {
	return sanitizeID(c.Broker.ClientID)
}

// StateTopic is where the JSON state document is published.
func (c Config) StateTopic() string { return c.HomeAssistant.BaseTopic + "/state" }

// AvailabilityTopic is the retained MQTT last-will topic.
func (c Config) AvailabilityTopic() string {
	return c.HomeAssistant.BaseTopic + "/availability"
}

// BrokerAddress returns host:port for display and dialing.
func (c Config) BrokerAddress() string {
	return c.Broker.Host + ":" + strconv.Itoa(c.Broker.Port)
}

// Load reads a configuration from path. The bool is false when the file does
// not exist; in that case the returned configuration is Default().
func Load(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		c := Default()
		c.Normalize()
		return c, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read config: %w", err)
	}
	c := Default()
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, true, fmt.Errorf("decode config: %w", err)
	}
	c.Normalize()
	return c, true, nil
}

// Save writes the configuration atomically with owner-only permissions.
func (c Config) Save(path string) error {
	c.Normalize()
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

var nonID = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizeID turns a name into lower-case ASCII for MQTT topics and HA ids.
func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonID.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-_")
	}
	if s == "" {
		return "mac2mqtt"
	}
	return s
}

// SanitizeID exposes the same normalization for UI previews.
func SanitizeID(s string) string { return sanitizeID(s) }

func hostname() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "Mac"
	}
	return strings.TrimSpace(h)
}

func deviceName(host string) string {
	if name, _, ok := strings.Cut(host, "."); ok && name != "" {
		host = name
	}
	if host == "" {
		return "Mac"
	}
	return host
}
