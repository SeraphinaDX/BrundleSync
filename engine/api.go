// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors

package engine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// These adapters deliberately expose configuration and lifecycle, not widgets
// or Android types. CLI, desktop and mobile all share the same validation.

func DefaultConfigPath() string                           { return configPath() }
func GenerateSecret() string                              { return randomHex(32) }
func LoadConfigForMode(name, mode string) (Config, error) { return readConfig(name, mode) }
func LoadConfig(name string) (Config, error)              { return readConfig(name, "client") }
func DecodeConfig(text string) (Config, error) {
	var cfg Config
	meta, err := toml.Decode(text, &cfg)
	if err != nil {
		return cfg, err
	}
	if len(meta.Undecoded()) != 0 {
		return cfg, errors.New("unknown TOML settings")
	}
	return validateConfig(cfg, "client")
}

func EncodeConfig(cfg Config) (string, error) {
	var out bytes.Buffer
	err := toml.NewEncoder(&out).Encode(cfg)
	return out.String(), err
}

// AppendFolderConfig leaves the rest of an existing TOML document untouched.
func AppendFolderConfig(text string, folder FolderConfig) (string, error) {
	var out bytes.Buffer
	if err := toml.NewEncoder(&out).Encode(struct {
		Folders []FolderConfig `toml:"folders"`
	}{[]FolderConfig{folder}}); err != nil {
		return "", err
	}
	result := strings.TrimSpace(text) + "\n\n" + out.String()
	if _, err := DecodeConfig(result); err != nil {
		return "", err
	}
	return result, nil
}

// SaveConfigText preserves the user's comments and formatting, validates first,
// and replaces atomically. Call only while the local sync engine is stopped.
func SaveConfigText(name, text string) error {
	if _, err := DecodeConfig(text); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(text); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}

// RequestSync wakes a running engine and requests a metadata scan even if its
// watcher has seen no events. It does not discard verified content hashes.
func (c *Client) RequestSync() {
	c.forceScan.Store(true)
	select {
	case c.wake <- struct{}{}:
	default:
	}
}
