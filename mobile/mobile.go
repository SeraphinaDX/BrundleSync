// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors

// Package mobile is the small gomobile-compatible API used by Android. No
// networking, cryptography or TOML parsing is reimplemented in Kotlin.
package mobile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"douchesync/engine"
)

// DecodeSetup validates the versioned QR payload in the shared engine.
func DecodeSetup(code string) (string, error) {
	s, err := engine.DecodeAndroidSetup(code)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(s)
	return string(data), err
}

func Version() string     { return engine.Version() }
func NewDeviceID() string { return "phone-" + engine.GenerateSecret()[:12] }

type settings struct {
	Scope          string `json:"scope"`
	DeviceID       string `json:"deviceID"`
	DiscoveryURL   string `json:"discoveryURL"`
	DiscoveryToken string `json:"discoveryToken"`
	FolderID       string `json:"folderID"`
	Secret         string `json:"secret"`
	CameraURI      string `json:"cameraURI"`
	Automatic      bool   `json:"automatic"`
	Unmetered      bool   `json:"unmetered"`
	Charging       bool   `json:"charging"`
}

func Settings(configPath string) (string, error) {
	cfg, err := engine.LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	if len(cfg.Folders) != 1 {
		return "", errors.New("Android camera backup requires exactly one destination folder")
	}
	f := cfg.Folders[0]
	data, err := json.Marshal(settings{configScope(cfg), cfg.Client.DeviceID, cfg.Client.DiscoveryURL, cfg.Client.DiscoveryToken, f.ID, f.Secret, cfg.Android.CameraURI, cfg.Android.Automatic, cfg.Android.Unmetered, cfg.Android.Charging})
	return string(data), err
}

func Configure(configPath, deviceID, discoveryURL, token, folderID, secret, cameraURI string, automatic, unmetered, charging bool) error {
	if len(cameraURI) < 10 || cameraURI[:10] != "content://" {
		return errors.New("choose a camera folder using Android's folder picker")
	}
	base := filepath.Dir(configPath)
	staging := filepath.Join(base, "outgoing")
	if err := os.MkdirAll(staging, 0700); err != nil {
		return err
	}
	cfg := engine.Config{Client: engine.ClientConfig{DeviceID: deviceID, Listen: "127.0.0.1:0", IdentityDir: filepath.Join(base, "identities"), DiscoveryURL: discoveryURL, DiscoveryToken: token, TransferTimeout: "5m"}, Folders: []engine.FolderConfig{{ID: folderID, Path: staging, Secret: secret}}, Android: engine.AndroidConfig{CameraURI: cameraURI, Automatic: automatic, Unmetered: unmetered, Charging: charging}}
	text, err := engine.EncodeConfig(cfg)
	if err != nil {
		return err
	}
	return engine.SaveConfigText(configPath, text)
}

type Session struct {
	sender           *engine.BackupSender
	ctx              context.Context
	cancel           context.CancelFunc
	scope, cameraURI string
	maxFileSize      int64
}

func NewSession(configPath string) (*Session, error) {
	cfg, err := engine.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	if len(cfg.Folders) != 1 {
		return nil, errors.New("configure one Android backup folder")
	}
	f := cfg.Folders[0]
	sender, err := engine.NewBackupSender(cfg, f.ID)
	if err != nil {
		return nil, err
	}
	// Include destination credentials, device identity and source selection in
	// the ledger scope. Changing any of them must not reuse an old success.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	return &Session{sender: sender, ctx: ctx, cancel: cancel, scope: configScope(cfg), cameraURI: cfg.Android.CameraURI, maxFileSize: f.MaxFileSize}, nil
}
func (s *Session) MaxFileSize() int64 { return s.maxFileSize }
func (s *Session) Scope() string      { return s.scope }
func (s *Session) CameraURI() string  { return s.cameraURI }
func (s *Session) Cancel()            { s.cancel() }
func SetAutomatic(configPath string, enabled bool) error {
	cfg, err := engine.LoadConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Android.Automatic = enabled
	text, err := engine.EncodeConfig(cfg)
	if err != nil {
		return err
	}
	return engine.SaveConfigText(configPath, text)
}
func (s *Session) Upload(stagedPath, displayName string, modifiedMillis int64) (string, error) {
	modified := time.Time{}
	if modifiedMillis > 0 {
		modified = time.UnixMilli(modifiedMillis)
	}
	receipt, err := s.sender.Upload(s.ctx, stagedPath, displayName, modified)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	return string(data), err
}

// UploadDescriptor takes ownership of a detached ParcelFileDescriptor. Close
// happens here on every return path, including cancellation and validation errors.
func (s *Session) UploadDescriptor(descriptor int64, displayName string, modifiedMillis int64) (string, error) {
	if descriptor < 0 {
		return "", errors.New("invalid camera file descriptor")
	}
	in := os.NewFile(uintptr(descriptor), "camera-source")
	if in == nil {
		return "", errors.New("invalid camera file")
	}
	defer in.Close()
	modified := time.Time{}
	if modifiedMillis > 0 {
		modified = time.UnixMilli(modifiedMillis)
	}
	receipt, err := s.sender.UploadOpenFile(s.ctx, in, displayName, modified)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	return string(data), err
}

func configScope(cfg engine.Config) string {
	f := cfg.Folders[0]
	key, _ := json.Marshal([]string{cfg.Client.DeviceID, cfg.Client.DiscoveryURL, cfg.Client.DiscoveryToken, f.ID, f.Secret, cfg.Android.CameraURI})
	hash := sha256.Sum256(key)
	return hex.EncodeToString(hash[:])
}
