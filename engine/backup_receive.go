// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

const backupChunkSize = 1 << 20

func (c *Client) serveBackup(w http.ResponseWriter, r *http.Request, f *Folder) {
	if !f.cfg.ReceiveBackups {
		http.Error(w, "enable receive_backups for this folder on the desktop", 403)
		return
	}
	q := r.URL.Query()
	p, err := backupPath(q.Get("device"), q.Get("name"), q.Get("hash"))
	size, e1 := strconv.ParseInt(q.Get("size"), 10, 64)
	modified, e2 := strconv.ParseInt(q.Get("modified"), 10, 64)
	if err != nil || e1 != nil || e2 != nil || modified < 0 || size < 0 || size > f.cfg.MaxFileSize || f.ignored(p) {
		http.Error(w, "invalid backup metadata, ignore rule, or file size", 400)
		return
	}
	select {
	case c.backupSlots <- struct{}{}:
		defer func() { <-c.backupSlots }()
	case <-r.Context().Done():
		return
	}
	key := digest([]byte(p))
	part := ".douchesync/incoming/" + key + ".part"
	release, err := f.lockIncoming(r.Context(), key)
	if err != nil {
		return
	}
	defer release()
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(c.transferTimeout))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(c.transferTimeout))
	f.mu.Lock()
	receipt, err := f.backupReceiptLocked(r.Context(), p, q.Get("hash"), size)
	f.mu.Unlock()
	if err != nil {
		http.Error(w, "backup unavailable or existing contents changed", 409)
		return
	}
	if receipt != nil {
		f.root.Remove(part)
		jsonResponse(w, receipt)
		return
	}
	if r.Method == http.MethodGet {
		info, err := f.root.Lstat(part)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > size {
			http.Error(w, "invalid incomplete backup", 409)
			return
		}
		if info.Size() == size {
			c.finishBackup(w, r, f, part, p, q.Get("hash"), size, modified)
			return
		}
		jsonResponse(w, BackupReceipt{Path: p, Hash: q.Get("hash"), Size: size, Offset: info.Size()})
		return
	}
	offset := int64(0)
	chunkSize := size
	chunkHash := q.Get("hash")
	if q.Has("offset") {
		var e3, e4 error
		offset, e3 = strconv.ParseInt(q.Get("offset"), 10, 64)
		chunkSize, e4 = strconv.ParseInt(q.Get("chunk_size"), 10, 64)
		chunkHash = q.Get("chunk_hash")
		if e3 != nil || e4 != nil {
			http.Error(w, "invalid chunk metadata", 400)
			return
		}
	}
	hashBytes, hashErr := hex.DecodeString(chunkHash)
	if offset < 0 || offset > size || chunkSize < 0 || chunkSize > backupChunkSize || chunkSize > size-offset || r.ContentLength != chunkSize || hashErr != nil || len(hashBytes) != 32 {
		http.Error(w, "invalid chunk range, length, or hash", 400)
		return
	}
	// Verify the bounded chunk before touching resumable state. An interrupted
	// or corrupt request cannot poison the already acknowledged prefix.
	tmp := ".douchesync/transfers/chunk-" + randomHex(16)
	out, err := f.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		http.Error(w, "cannot create chunk temporary file", 500)
		return
	}
	defer f.root.Remove(tmp)
	transfer := f.status.begin("Camera backup chunk", f.cfg.ID, p, size)
	transfer.bytes.Store(offset)
	defer func() { f.status.finish(transfer, err) }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(progressReader{Reader: contextReader{ctx: r.Context(), in: r.Body}, transfer: transfer}, chunkSize+1))
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil || n != chunkSize || hex.EncodeToString(hash.Sum(nil)) != chunkHash {
		err = errors.New("chunk verification failed")
		http.Error(w, err.Error(), 400)
		return
	}
	file, err := f.openIncoming(part)
	if err != nil {
		http.Error(w, "cannot open resumable backup (limit: 64 unfinished files)", 500)
		return
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > size {
		file.Close()
		http.Error(w, "invalid incomplete backup", 409)
		return
	}
	if info.Size() != offset {
		file.Close()
		if info.Size() == size {
			c.finishBackup(w, r, f, part, p, q.Get("hash"), size, modified)
			return
		}
		jsonResponse(w, BackupReceipt{Path: p, Hash: q.Get("hash"), Size: size, Offset: info.Size()})
		return
	}
	in, err := f.root.Open(tmp)
	if err == nil {
		_, err = file.Seek(offset, io.SeekStart)
		if err == nil {
			_, err = io.Copy(file, contextReader{ctx: r.Context(), in: in})
		}
		in.Close()
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = f.syncDirectory(".douchesync/incoming")
	}
	if err != nil {
		http.Error(w, "cannot commit backup chunk", 500)
		return
	}
	if offset+chunkSize < size {
		jsonResponse(w, BackupReceipt{Path: p, Hash: q.Get("hash"), Size: size, Offset: offset + chunkSize})
		return
	}
	c.finishBackup(w, r, f, part, p, q.Get("hash"), size, modified)
}

// The upload lock is held, but f.mu remains free while hashing a large video.
// Incomplete files never enter the manifest, even across receiver restarts.
func (c *Client) finishBackup(w http.ResponseWriter, r *http.Request, f *Folder, part, p, hash string, size, modified int64) {
	entry, _, err := f.hashFileObserved(r.Context(), part)
	if err != nil {
		http.Error(w, "backup verification interrupted", 500)
		return
	}
	if entry.Hash != hash || entry.Size != size {
		f.root.Remove(part)
		http.Error(w, "complete backup hash mismatch; retry from the beginning", 400)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	receipt, err := f.backupReceiptLocked(r.Context(), p, hash, size)
	if err != nil {
		http.Error(w, "backup history unavailable or destination changed", 409)
		return
	}
	if receipt != nil {
		f.root.Remove(part)
		jsonResponse(w, receipt)
		return
	}
	if _, known := f.state.Entries[p]; !known && len(f.state.Entries) >= maxEntries {
		http.Error(w, "folder history is full", 413)
		return
	}
	if err = r.Context().Err(); err != nil {
		return
	}
	if err = f.checkAncestors(p); err == nil {
		err = f.root.MkdirAll(filepath.FromSlash(path.Dir(p)), 0700)
	}
	if err == nil {
		err = f.checkAncestors(p)
	}
	if err == nil {
		err = f.root.Chmod(part, 0644)
	}
	if err == nil && modified > 0 {
		t := time.Unix(0, modified)
		err = f.root.Chtimes(part, t, t)
	}
	if err == nil {
		err = f.installBackupLocked(r.Context(), part, filepath.FromSlash(p))
	}
	if err != nil {
		http.Error(w, "backup install failed", 500)
		return
	}
	info, err := f.root.Stat(filepath.FromSlash(p))
	if err == nil {
		entry.ModTime, entry.Mode = info.ModTime().UnixNano(), uint32(info.Mode().Perm())
		err = f.observeLocked(p, &entry)
		if err == nil {
			err = f.persistEntryLocked(p, true)
		}
	}
	if err != nil {
		f.healthy = false
		http.Error(w, "backup history commit failed", 500)
		return
	}
	if err = f.syncBackupDirectories(p); err != nil {
		f.healthy = false
		http.Error(w, "backup directory commit failed", 500)
		return
	}
	f.root.Remove(part)
	if current, e := f.root.Lstat(filepath.FromSlash(p)); e == nil {
		if stamp, e := f.changeStamp(p, current); e == nil {
			f.hashCache[p] = cachedHash{entry: entry, info: current, stamp: stamp, verified: time.Now()}
		}
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	jsonResponse(w, BackupReceipt{Path: p, Hash: hash, Size: size, Offset: size, Complete: true})
}

// Link publishes the already-verified inode atomically without replacing an
// existing file. Filesystems without hardlinks use an exclusive creation and a
// cancellable copy under the folder lock, so peers never see a partial version.
func (f *Folder) installBackupLocked(ctx context.Context, tmp, destination string) error {
	if err := f.root.Link(tmp, destination); err == nil {
		return nil
	} else if errors.Is(err, os.ErrExist) {
		return err
	}
	in, err := f.root.Open(tmp)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := f.root.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			f.root.Remove(destination)
		}
	}()
	if _, err = io.Copy(out, contextReader{ctx: ctx, in: in}); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = f.root.Chtimes(destination, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	ok = true
	return nil
}

// Caller holds f.mu. An acknowledgement verifies disk contents, rather than
// trusting a filename or a receipt left over from a file that was changed.
func (f *Folder) backupReceiptLocked(ctx context.Context, p, hash string, size int64) (*BackupReceipt, error) {
	if !f.healthy {
		return nil, errors.New("folder state unavailable")
	}
	if err := f.checkAncestors(p); err != nil {
		return nil, err
	}
	if _, err := f.root.Lstat(filepath.FromSlash(p)); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if err := f.refreshContextLocked(ctx, p, false); err != nil {
		return nil, err
	}
	e := f.state.Entries[p]
	if e.Deleted || e.Missing || e.Hash != hash || e.Size != size {
		return nil, errors.New("backup path is occupied by different contents")
	}
	// refresh also repairs history when a crash occurred just after installation.
	if f.dirty || f.pendingUpdates != 0 {
		if err := f.saveLocked(); err != nil {
			return nil, err
		}
	}
	if err := f.syncBackupDirectories(p); err != nil {
		return nil, err
	}
	return &BackupReceipt{Path: p, Hash: hash, Size: size, Complete: true, Offset: size}, nil
}
