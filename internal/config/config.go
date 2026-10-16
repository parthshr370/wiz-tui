// Package config keeps wiz's small amount of state in ~/.config/wiz:
// the optional home signing key, and known bulbs with their names.
package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const signingKeyBytes = 16

// Known is one remembered bulb.
type Known struct {
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
}

// Store is the contents of bulbs.json.
type Store struct {
	Default string           `json:"default,omitempty"` // MAC of the bulb commands target
	Bulbs   map[string]Known `json:"bulbs"`             // keyed by MAC
	// Favourites are preset keys for the TUI's first tab, in order. nil means
	// never edited, so the built-in picks apply.
	Favourites []string `json:"favourites"`
}

// Dir is ~/.config/wiz, or $WIZ_CONFIG_DIR when set.
func Dir() (string, error) {
	if dir := os.Getenv("WIZ_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".config", "wiz"), nil
}

// SigningKey returns the home udp_signing_key from <dir>/key, or nil if the
// file does not exist.
func SigningKey() ([]byte, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "key")
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(text)))
	if err != nil || len(key) != signingKeyBytes {
		return nil, fmt.Errorf("%s must hold the 32-hex-character udp_signing_key", path)
	}
	return key, nil
}

// Load reads bulbs.json; a missing file is an empty store.
func Load() (Store, error) {
	store := Store{Bulbs: map[string]Known{}}
	dir, err := Dir()
	if err != nil {
		return store, err
	}
	path := filepath.Join(dir, "bulbs.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &store); err != nil {
		return store, fmt.Errorf("parse %s: %w", path, err)
	}
	if store.Bulbs == nil {
		store.Bulbs = map[string]Known{}
	}
	return store, nil
}

// Save writes bulbs.json atomically.
func (s Store) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bulbs.json: %w", err)
	}
	path := filepath.Join(dir, "bulbs.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Remember records a bulb's current IP, keeping its name.
func (s *Store) Remember(mac, ip string) {
	known := s.Bulbs[mac]
	known.IP = ip
	s.Bulbs[mac] = known
	if s.Default == "" {
		s.Default = mac
	}
}

// Name returns the user's name for a bulb, or "".
func (s Store) Name(mac string) string { return s.Bulbs[mac].Name }

// Rename sets or clears a bulb's name.
func (s *Store) Rename(mac, name string) {
	known := s.Bulbs[mac]
	known.Name = strings.TrimSpace(name)
	s.Bulbs[mac] = known
}

// Resolve finds a MAC from a MAC, IP or name. An empty query means the default.
func (s Store) Resolve(query string) (string, bool) {
	if query == "" {
		return s.Default, s.Default != ""
	}
	for mac, known := range s.Bulbs {
		if strings.EqualFold(mac, query) || known.IP == query || strings.EqualFold(known.Name, query) {
			return mac, true
		}
	}
	return "", false
}
