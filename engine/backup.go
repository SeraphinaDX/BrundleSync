// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Backups have a separate, append-only protocol. Phone deletions never become
// folder tombstones, and no remote data is pulled back onto the phone. The
// content hash in each filename makes retries deterministic and preserves
// different photos that happen to share a camera filename.
func backupPath(device, name, hash string) (string, error) {
	decoded, err := hex.DecodeString(hash)
	if !validID.MatchString(device) || err != nil || len(decoded) != 32 || hash != strings.ToLower(hash) {
		return "", errors.New("invalid backup identity or hash")
	}
	if name == "" || len(name) > 1024 || strings.ContainsAny(name, "/\\\x00") {
		return "", errors.New("invalid camera filename")
	}
	ext := strings.ToLower(path.Ext(name))
	if len(ext) > 16 {
		ext = ""
	}
	clean := func(s string) string {
		var out strings.Builder
		for _, r := range s {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
				out.WriteRune(r)
			} else {
				out.WriteByte('_')
			}
		}
		return out.String()
	}
	stem := strings.Trim(clean(strings.TrimSuffix(name, path.Ext(name))), ". ")
	if len(stem) > 80 {
		stem = stem[:80]
	}
	if stem == "" {
		stem = "photo"
	}
	return "Camera/" + device + "/" + stem + "-" + hash + clean(ext), nil
}

type BackupReceipt struct {
	Path     string `json:"path"`
	Hash     string `json:"hash"`
	Size     int64  `json:"size"`
	Peer     string `json:"peer,omitempty"`
	Complete bool   `json:"complete"`
	Offset   int64  `json:"offset"`
}

// BackupSender is an outbound-only client. It queries signed discovery entries,
// then uploads directly to a pinned peer; it never registers a phone listener.
type BackupSender struct {
	cfg       ClientConfig
	folder    FolderConfig
	discovery *http.Client
}

func NewBackupSender(cfg Config, folderID string) (*BackupSender, error) {
	validated, err := validateConfig(cfg, "client")
	if err != nil {
		return nil, err
	}
	for _, f := range validated.Folders {
		if f.ID == folderID {
			return &BackupSender{cfg: validated.Client, folder: f, discovery: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("discovery redirects forbidden") }}}, nil
		}
	}
	return nil, errors.New("backup folder not configured")
}

func (s *BackupSender) Upload(ctx context.Context, filename, displayName string, modified time.Time) (BackupReceipt, error) {
	var empty BackupReceipt
	in, err := os.Open(filename)
	if err != nil {
		return empty, err
	}
	defer in.Close()
	return s.UploadOpenFile(ctx, in, displayName, modified)
}

// UploadOpenFile borrows a seekable regular file. Android supplies the file
// descriptor obtained through its native folder grant, with no raw path lookup
// or second copy of ordinary camera files. Ownership remains with the caller.
func (s *BackupSender) UploadOpenFile(ctx context.Context, in *os.File, displayName string, modified time.Time) (BackupReceipt, error) {
	var empty BackupReceipt
	before, err := in.Stat()
	if err != nil {
		return empty, err
	}
	if !before.Mode().IsRegular() || before.Size() > s.folder.MaxFileSize {
		return empty, errors.New("backup source is not a regular file or exceeds max_file_size")
	}
	beforeStamp := fileChangeStamp(in, before)
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return empty, errors.New("camera source is not seekable")
	}
	h := sha256.New()
	n, err := io.Copy(h, contextReader{ctx: ctx, in: in})
	if err != nil {
		return empty, err
	}
	after, err := in.Stat()
	if err != nil {
		return empty, err
	}
	afterStamp := fileChangeStamp(in, after)
	if !sameFileStat(before, after) || n != before.Size() || beforeStamp.known && afterStamp.known && beforeStamp != afterStamp {
		return empty, errors.New("backup source changed while hashing")
	}
	hash := hex.EncodeToString(h.Sum(nil))
	destination, err := backupPath(s.cfg.DeviceID, displayName, hash)
	if err != nil {
		return empty, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.DiscoveryURL+"/v1/peers?room="+roomID(s.folder), nil)
	if err != nil {
		return empty, err
	}
	request.Header.Set("Authorization", "Bearer "+s.cfg.DiscoveryToken)
	response, err := s.discovery.Do(request)
	if err != nil {
		return empty, err
	}
	var peers []Announcement
	err = requireStatus(response, 200)
	if err == nil {
		err = decodeLimited(response, 8<<20, &peers)
	}
	response.Body.Close()
	if err != nil {
		return empty, err
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Device < peers[j].Device })
	modifiedNanos := modified.UnixNano()
	if modified.IsZero() {
		modifiedNanos = 0
	}
	q := url.Values{"room": {roomID(s.folder)}, "device": {s.cfg.DeviceID}, "name": {displayName}, "hash": {hash}, "size": {strconv.FormatInt(n, 10)}, "modified": {strconv.FormatInt(modifiedNanos, 10)}}
	uri := "/v1/backup?" + q.Encode()
	timeout, _ := duration(s.cfg.TransferTimeout, 30*time.Minute)
	var problems []error
	for _, peer := range peers {
		if !peer.verified(s.folder) || peer.Device == s.cfg.DeviceID {
			continue
		}
		for _, endpoint := range peer.endpoints() {
			if ctx.Err() != nil {
				return empty, ctx.Err()
			}
			peer.URL = endpoint
			client := pinnedClient(peer, timeout)
			client.Transport.(*http.Transport).ResponseHeaderTimeout = timeout
			receipt, err := s.uploadToPeer(ctx, client, peer, in, uri, destination, hash, n)
			client.CloseIdleConnections()
			if err == nil {
				latest, checkErr := in.Stat()
				if checkErr != nil {
					return empty, checkErr
				}
				stamp := fileChangeStamp(in, latest)
				if !sameFileStat(after, latest) || afterStamp.known && stamp.known && afterStamp != stamp {
					return empty, errors.New("camera source changed during transfer; backup will retry")
				}
				receipt.Peer = peer.Device
				return receipt, nil
			}
			problems = append(problems, fmt.Errorf("%s: %w", peer.Device, err))
		}
	}
	if len(problems) == 0 {
		return empty, errors.New("no authenticated desktop peers discovered for the backup folder")
	}
	return empty, fmt.Errorf("camera backup failed (desktop must run 0.5.0+ with receive_backups=true): %w", errors.Join(problems...))
}

func (s *BackupSender) uploadToPeer(ctx context.Context, client *http.Client, peer Announcement, in *os.File, uri, destination, hash string, size int64) (BackupReceipt, error) {
	result := BackupReceipt{Path: destination, Hash: hash, Size: size}
	probe, err := http.NewRequestWithContext(ctx, http.MethodGet, peer.URL+uri, nil)
	if err != nil {
		return result, err
	}
	signRequest(probe, s.folder.Secret)
	response, err := client.Do(probe)
	if err != nil {
		return result, err
	}
	if response.StatusCode != http.StatusNotFound {
		err = requireStatus(response, 200)
		if err == nil {
			err = decodeLimited(response, 4096, &result)
		}
	}
	response.Body.Close()
	if err != nil {
		return result, err
	}
	valid := func(v BackupReceipt) bool {
		return v.Path == destination && v.Hash == hash && v.Size == size && v.Offset >= 0 && v.Offset <= size && (!v.Complete || v.Offset == size)
	}
	if !valid(result) {
		return result, errors.New("peer returned an invalid backup receipt")
	}
	buffer := make([]byte, backupChunkSize)
	for !result.Complete {
		offset := result.Offset
		length := min(int64(backupChunkSize), size-offset)
		data := buffer[:int(length)]
		if _, err = io.ReadFull(contextReader{ctx: ctx, in: io.NewSectionReader(in, offset, length)}, data); err != nil {
			return result, err
		}
		chunkURI := uri + "&offset=" + strconv.FormatInt(offset, 10) + "&chunk_size=" + strconv.FormatInt(length, 10) + "&chunk_hash=" + digest(data)
		var body io.Reader
		if length > 0 {
			body = bytes.NewReader(data)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, peer.URL+chunkURI, body)
		if err != nil {
			return result, err
		}
		request.ContentLength = length
		signRequest(request, s.folder.Secret)
		response, err = client.Do(request)
		if err != nil {
			return result, err
		}
		err = requireStatus(response, 200)
		if err == nil {
			err = decodeLimited(response, 4096, &result)
		}
		response.Body.Close()
		if err != nil {
			return result, err
		}
		if !valid(result) || !result.Complete && result.Offset <= offset {
			return result, errors.New("peer returned an invalid chunk checkpoint")
		}
	}
	return result, nil
}
