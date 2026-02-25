/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package ttrpcutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "github.com/containerd/containerd/api/services/ttrpc/events/v1"
	"github.com/containerd/containerd/v2/pkg/dialer"
	"github.com/containerd/ttrpc"
)

const ttrpcDialTimeout = 5 * time.Second

// ttrpcConnector creates a new TTRPC client connection.
type ttrpcConnector func() (*ttrpc.Client, error)

// Client manages a TTRPC connection to the containerd daemon (plugins, events).
type Client struct {
	mu        sync.Mutex
	connector ttrpcConnector
	client    *ttrpc.Client
	closed    bool
}

// NewClient creates a TTRPC client for the given address. Connection is
// established lazily on first use. Uses ttrpcDialTimeout for dial.
func NewClient(address string, opts ...ttrpc.ClientOpts) (*Client, error) {
	connector := func() (*ttrpc.Client, error) {
		ctx, _ := context.WithTimeout(context.Background(), ttrpcDialTimeout)
		conn, err := dialer.ContextDialer(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}

		client := ttrpc.NewClient(conn, opts...)
		return client, nil
	}

	return &Client{
		connector: connector,
	}, nil
}

// Reconnect closes the current connection and establishes a new one.
func (c *Client) Reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connector == nil {
		return errors.New("unable to reconnect to containerd, no connector available")
	}

	if c.closed {
		return errors.New("client is closed")
	}

	if c.client != nil {
		if err := c.client.Close(); err != nil {
			return err
		}
	}

	client, err := c.connector()
	if err != nil {
		return err
	}

	c.client = client
	return nil
}

// EventsService returns the TTRPC events service client.
func (c *Client) EventsService() (v1.TTRPCEventsService, error) {
	client, err := c.Client()
	if err != nil {
		return nil, err
	}
	return v1.NewTTRPCEventsClient(client), nil
}

// Client returns the underlying TTRPC client, connecting if needed.
func (c *Client) Client() (*ttrpc.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		client, err := c.connector()
		if err != nil {
			return nil, err
		}
		c.client = client
	}
	return c.client, nil
}

// Close closes the TTRPC connection. Safe to call multiple times.
func (c *Client) Close() error {
	c.closed = true
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
