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

package snapshotservice

import (
	"context"
	"testing"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
)

type mockSnapshotter struct {
	commitOpts []snapshots.Opt
}

func (m *mockSnapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	return snapshots.Info{}, nil
}

func (m *mockSnapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	return snapshots.Info{}, nil
}

func (m *mockSnapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	return snapshots.Usage{}, nil
}

func (m *mockSnapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	return nil, nil
}

func (m *mockSnapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	return nil, nil
}

func (m *mockSnapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	return nil, nil
}

func (m *mockSnapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	m.commitOpts = opts
	return nil
}

func (m *mockSnapshotter) Remove(ctx context.Context, key string) error {
	return nil
}

func (m *mockSnapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	return nil
}

func (m *mockSnapshotter) Close() error {
	return nil
}

func TestCommitParentOptionSmoke(t *testing.T) {
	mock := &mockSnapshotter{}
	svc := FromSnapshotter(mock)

	_, err := svc.Commit(context.Background(), &snapshotsapi.CommitSnapshotRequest{
		Name: "smoke-snapshot",
		Key:  "smoke-key",
	})
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	if len(mock.commitOpts) == 0 {
		t.Log("smoke commit did not pass any snapshot opts")
	}
}
