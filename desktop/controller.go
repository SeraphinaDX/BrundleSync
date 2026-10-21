// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors
package main

import (
	"context"
	"errors"
	"sync"

	"brundlesync/engine"
)

// The controller owns exactly one engine lifetime. Stopping waits for all IO
// and identity locks to close before a later Start is permitted.
type controller struct {
	mu             sync.Mutex
	client         *engine.Client
	cancel         context.CancelFunc
	done           chan struct{}
	phase, problem string
	last           engine.Status
}

func (c *controller) start(configPath string) error {
	c.mu.Lock()
	if c.done != nil {
		c.mu.Unlock()
		return errors.New("sync is already running or stopping")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel, c.done, c.phase, c.problem = cancel, make(chan struct{}), "starting", ""
	c.last = engine.Status{}
	done := c.done
	c.mu.Unlock()
	go func() {
		cfg, err := engine.LoadConfig(configPath)
		var client *engine.Client
		if err == nil && ctx.Err() == nil {
			client, err = engine.NewClient(cfg)
		}
		if client != nil {
			c.mu.Lock()
			c.client = client
			c.phase = "running"
			c.mu.Unlock()
			if ctx.Err() == nil {
				err = client.Run(ctx)
			}
			client.Close()
		}
		c.mu.Lock()
		if client != nil {
			c.last = client.Snapshot()
			c.last.Phase = "stopped"
			for i := range c.last.Peers {
				c.last.Peers[i].State = "stopped"
			}
		}
		c.client, c.cancel, c.done, c.phase = nil, nil, nil, "stopped"
		if err != nil {
			c.problem = err.Error()
		}
		close(done)
		c.mu.Unlock()
	}()
	return nil
}
func (c *controller) stop() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	c.phase = "stopping"
	c.cancel()
	return c.done
}
func (c *controller) snapshot() (engine.Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.last
	s.Phase = c.phase
	if c.problem != "" {
		s.LastError = c.problem
	}
	if s.Phase == "" {
		s.Phase = "stopped"
	}
	if c.client != nil {
		s = c.client.Snapshot()
		if c.phase == "stopping" {
			s.Phase = "stopping"
		}
	}
	return s, c.done != nil
}
func (c *controller) syncNow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		c.client.RequestSync()
	}
}
