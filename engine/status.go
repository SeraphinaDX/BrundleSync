// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors

package engine

import (
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Status struct {
	Device, Address, Phase, LastError string
	LastCycle                         time.Time
	Folders                           []FolderStatus
	Peers                             []PeerStatus
	Transfers                         []TransferStatus
	Events                            []Event
}
type FolderStatus struct {
	ID, Path                      string
	Files, Deleted                int
	Busy, Healthy, ReceiveBackups bool
}
type PeerStatus struct{ Folder, Device, Address, State string }
type TransferStatus struct {
	Direction, Folder, Path string
	Bytes, Total            int64
}
type Event struct {
	Time    time.Time
	Message string
}

type liveTransfer struct {
	TransferStatus
	bytes atomic.Int64
}
type statusTracker struct {
	mu               sync.Mutex
	phase, lastError string
	discoveryError   string
	lastCycle        time.Time
	active           map[*liveTransfer]bool
	events           []Event
}

func (s *statusTracker) discoveryFinished(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	problem := ""
	if err != nil {
		problem = "Discovery: " + err.Error()
	}
	if problem != "" && problem != s.discoveryError {
		s.eventLocked(problem)
	}
	s.discoveryError = problem
}

func (s *statusTracker) eventLocked(message string) {
	s.events = append(s.events, Event{Time: time.Now(), Message: message})
	if len(s.events) > 100 {
		s.events = s.events[len(s.events)-100:]
	}
}
func (s *statusTracker) setPhase(phase string) { s.mu.Lock(); s.phase = phase; s.mu.Unlock() }
func (s *statusTracker) cycleFinished(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase, s.lastCycle = "idle", time.Now()
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	if problem != s.lastError && problem != "" {
		s.eventLocked(problem)
	}
	s.lastError = problem
}
func (s *statusTracker) begin(direction, folder, path string, total int64) *liveTransfer {
	t := &liveTransfer{TransferStatus: TransferStatus{Direction: direction, Folder: folder, Path: path, Total: total}}
	if s != nil {
		s.mu.Lock()
		if s.active == nil {
			s.active = map[*liveTransfer]bool{}
		}
		s.active[t] = true
		s.mu.Unlock()
	}
	return t
}
func (s *statusTracker) finish(t *liveTransfer, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, t)
	message := t.Direction + " " + t.Folder + "/" + t.Path
	if err != nil {
		message += ": " + err.Error()
	} else {
		message += " verified"
	}
	s.eventLocked(message)
}

type progressReader struct {
	io.Reader
	transfer *liveTransfer
}

func (r progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.transfer.bytes.Add(int64(n))
	return n, err
}

// Snapshot never waits for a folder being hashed or mutated. This keeps the
// GUI responsive even during a full verification, with no UI callbacks in IO.
func (c *Client) Snapshot() Status {
	s := Status{Device: c.cfg.DeviceID}
	c.status.mu.Lock()
	s.Phase, s.LastError, s.LastCycle = c.status.phase, c.status.lastError, c.status.lastCycle
	if s.LastError == "" {
		s.LastError = c.status.discoveryError
	}
	s.Events = append([]Event(nil), c.status.events...)
	for t := range c.status.active {
		v := t.TransferStatus
		v.Bytes = t.bytes.Load()
		s.Transfers = append(s.Transfers, v)
	}
	c.status.mu.Unlock()
	c.mu.Lock()
	s.Address = c.localURL
	for room, f := range c.folders {
		for _, p := range c.peers[room] {
			state := "discovered"
			if checked, ok := c.peerChecks[room+"/"+p.Device]; ok {
				if checked.ok {
					state = "reachable"
				} else {
					state = "unreachable"
				}
			}
			s.Peers = append(s.Peers, PeerStatus{Folder: f.cfg.ID, Device: p.Device, Address: p.URL, State: state})
		}
	}
	c.mu.Unlock()
	for _, f := range c.folders {
		v := FolderStatus{ID: f.cfg.ID, Path: f.cfg.Path, ReceiveBackups: f.cfg.ReceiveBackups, Files: int(f.liveCount.Load()), Deleted: int(f.deletedCount.Load())}
		if f.mu.TryLock() {
			v.Healthy = f.healthy
			f.mu.Unlock()
		} else {
			v.Busy = true
		}
		s.Folders = append(s.Folders, v)
	}
	sort.Slice(s.Folders, func(i, j int) bool { return s.Folders[i].ID < s.Folders[j].ID })
	sort.Slice(s.Peers, func(i, j int) bool { return s.Peers[i].Folder+s.Peers[i].Device < s.Peers[j].Folder+s.Peers[j].Device })
	return s
}
