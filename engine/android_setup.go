// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const androidSetupPrefix = "DOUCHESYNC-CAMERA-1:"
const maxAndroidSetup = 2000

// AndroidSetup contains only the selected backup destination's credentials.
// It deliberately excludes desktop identity, local paths and other folders.
type AndroidSetup struct {
	DiscoveryURL   string `json:"discoveryURL"`
	DiscoveryToken string `json:"discoveryToken"`
	FolderID       string `json:"folderID"`
	Secret         string `json:"secret"`
}

func (s AndroidSetup) validate() error {
	_, err := validateConfig(Config{Client: ClientConfig{DeviceID: "phone-setup", Listen: "127.0.0.1:0", DiscoveryURL: s.DiscoveryURL, DiscoveryToken: s.DiscoveryToken}, Folders: []FolderConfig{{ID: s.FolderID, Path: ".", Secret: s.Secret}}}, "client")
	if err != nil {
		return errors.New("invalid Android setup destination or credentials")
	}
	return nil
}

func EncodeAndroidSetup(cfg Config, folderID string) (string, error) {
	for _, f := range cfg.Folders {
		if f.ID != folderID {
			continue
		}
		if !f.ReceiveBackups {
			return "", errors.New("enable receive_backups for this folder first")
		}
		s := AndroidSetup{cfg.Client.DiscoveryURL, cfg.Client.DiscoveryToken, f.ID, f.Secret}
		if err := s.validate(); err != nil {
			return "", err
		}
		b, _ := json.Marshal(s)
		code := androidSetupPrefix + base64.RawURLEncoding.EncodeToString(b)
		if len(code) > maxAndroidSetup {
			return "", errors.New("setup is too large for a QR code; use manual settings")
		}
		return code, nil
	}
	return "", errors.New("backup folder not found")
}

func DecodeAndroidSetup(code string) (AndroidSetup, error) {
	var s AndroidSetup
	invalid := errors.New("invalid or unsupported DoucheSync camera setup QR code")
	if len(code) > maxAndroidSetup || !strings.HasPrefix(code, androidSetupPrefix) {
		return s, invalid
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, androidSetupPrefix))
	if err != nil {
		return s, invalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		return AndroidSetup{}, invalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return AndroidSetup{}, invalid
	}
	if s.validate() != nil {
		return AndroidSetup{}, invalid
	}
	return s, nil
}
