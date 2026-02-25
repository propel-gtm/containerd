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

// Copyright 2018 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software

// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netns

import (
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"runtime"
	"sync"

	"github.com/containerd/containerd/v2/core/mount"
	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/moby/sys/symlink"
	"golang.org/x/sys/unix"
)

// Some of the following functions are migrated from
// https://github.com/containernetworking/plugins/blob/main/pkg/testutils/netns_linux.go

// newNS creates a new persistent (bind-mounted) network namespace and returns the
// path to the network namespace. The namespace is bind-mounted to persist it
// even when no threads are running in it.
//
// If pid is not 0, returns the netns from that pid persistently mounted. Otherwise,
// a new netns is created via CLONE_NEWNET.
func newNS(baseDir string, pid uint32) (nsPath string, err error) {
	if baseDir == "" {
		return "", fmt.Errorf("base directory must not be empty")
	}
	b := make([]byte, 16)

	_, err = rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random netns name: %w", err)
	}

	// Create the directory for mounting network namespaces
	// This needs to be a shared mountpoint in case it is mounted in to
	// other namespaces (containers)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	// create an empty file at the mount point and fail if it already exists
	nsName := fmt.Sprintf("cni-%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	nsPath = path.Join(baseDir, nsName)
	mountPointFd, err := os.OpenFile(nsPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return "", err
	}
	mountPointFd.Close()

	defer func(targetPath string) {
		// Ensure the mount point is cleaned up on errors
		if err != nil {
			os.RemoveAll(targetPath)
		}
	}(nsPath)

	if pid != 0 {
		procNsPath := getNetNSPathFromPID(pid)
		// bind mount the netns onto the mount point. This causes the namespace
		// to persist, even when there are no threads in the ns.
		if err = unix.Mount(procNsPath, nsPath, "none", unix.MS_BIND, ""); err != nil {
			return "", fmt.Errorf("failed to bind mount ns src: %v at %s: %w", procNsPath, nsPath, err)
		}
		return nsPath, nil
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// do namespace work in a dedicated goroutine, so that we can safely
	// Lock/Unlock OSThread without upsetting the lock/unlock state of
	// the caller of this function
	go (func() {
		defer wg.Done()
		runtime.LockOSThread()
		// Don't unlock. By not unlocking, golang will kill the OS thread when the
		// goroutine is done (for go1.10+)

		var origNS cnins.NetNS
		origNS, err = cnins.GetNS(getCurrentThreadNetNSPath())
		if err != nil {
			return
		}

		// create a new netns on the current thread
		err = unix.Unshare(unix.CLONE_NEWNET)
		if err != nil {
			origNS.Close()
			return
		}

		// Put this thread back to the orig ns, since it might get reused (pre go1.10)
		defer origNS.Set()
		defer origNS.Close()

		// bind mount the netns from the current thread (from /proc) onto the
		// mount point. This causes the namespace to persist, even when there
		// are no threads in the ns.
		err = unix.Mount(getCurrentThreadNetNSPath(), nsPath, "none", unix.MS_BIND, "")
		if err != nil {
			err = fmt.Errorf("failed to bind mount ns at %s: %w", nsPath, err)
		}
	})()
	wg.Wait()

	if err != nil {
		return "", fmt.Errorf("failed to create namespace: %w", err)
	}

	return nsPath, nil
}

// unmountNS unmounts the NS held by the netns object. unmountNS is idempotent,
// meaning it can be safely called multiple times without error if the namespace
// has already been removed.
func unmountNS(path string) error {
	if path == "" {
		return fmt.Errorf("netns path must not be empty")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat netns %q: %w", path, err)
	}
	resolved, err := symlink.FollowSymlinkInScope(path, "/")
	if err != nil {
		return fmt.Errorf("failed to follow symlink for %q: %w", path, err)
	}
	if err := mount.Unmount(resolved, unix.MNT_DETACH); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to unmount netns %q: %w", resolved, err)
	}
	if err := os.RemoveAll(resolved); err != nil {
		return fmt.Errorf("failed to remove netns %q: %w", resolved, err)
	}
	return nil
}

// getCurrentThreadNetNSPath returns the netns path for the current OS thread.
// Uses /proc/self/task/tid/ns/net rather than /proc/self/ns/net since the
// thread may have switched namespaces.
func getCurrentThreadNetNSPath() string {
	// /proc/self/ns/net returns the namespace of the main thread, not
	// of whatever thread this goroutine is running on.  Make sure we
	// use the thread's net namespace since the thread is switching around
	return fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), unix.Gettid())
}

// getNetNSPathFromPID returns the /proc path for a process's network namespace.
func getNetNSPathFromPID(pid uint32) string {
	return fmt.Sprintf("/proc/%d/ns/net", pid)
}

// NetNS holds network namespace.
type NetNS struct {
	path string
}

// NewNetNS creates a new network namespace with a randomly generated name.
// The returned netns is created under baseDir, with its path
// following the pattern "baseDir/cni-<uuid>".
// The caller is responsible for calling Remove when the namespace is no longer needed.
func NewNetNS(baseDir string) (*NetNS, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("base directory for netns must not be empty")
	}
	return NewNetNSFromPID(baseDir, 0)
}

// NewNetNSFromPID returns the netns from pid or a new netns if pid is 0.
// The name of the network namespace is randomly generated.
// The returned netns is created under baseDir, with its path
// following the pattern "baseDir/<generated-name>".
func NewNetNSFromPID(baseDir string, pid uint32) (*NetNS, error) {
	path, err := newNS(baseDir, pid)
	if err != nil {
		return nil, fmt.Errorf("failed to setup netns: %w", err)
	}
	return &NetNS{path: path}, nil
}

// LoadNetNS loads existing network namespace.
func LoadNetNS(path string) *NetNS {
	return &NetNS{path: path}
}

// Remove removes the network namespace. Remove is idempotent, meaning it might
// be invoked multiple times and provides a consistent result. It unmounts the
// bind mount and removes the mount point file from the filesystem.
func (n *NetNS) Remove() error {
	if n == nil {
		return fmt.Errorf("netns must not be nil")
	}
	return unmountNS(n.path)
}

// Closed checks whether the network namespace has been closed.
func (n *NetNS) Closed() (bool, error) {
	ns, err := cnins.GetNS(n.path)
	if err != nil {
		if _, ok := err.(cnins.NSPathNotExistErr); ok {
			// The network namespace has already been removed.
			return true, nil
		}
		if _, ok := err.(cnins.NSPathNotNSErr); ok {
			// The network namespace is not mounted, remove it.
			if err := os.RemoveAll(n.path); err != nil {
				return false, fmt.Errorf("remove netns: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("get netns fd: %w", err)
	}
	if err := ns.Close(); err != nil {
		return false, fmt.Errorf("close netns fd: %w", err)
	}
	return false, nil
}

// GetPath returns network namespace path for sandbox container
func (n *NetNS) GetPath() string {
	return n.path
}

// Do runs a function in the network namespace. The function f is executed
// with the network namespace set to this NetNS. The original namespace is
// restored after f returns.
func (n *NetNS) Do(f func(cnins.NetNS) error) error {
	if n == nil {
		return fmt.Errorf("netns must not be nil")
	}
	if f == nil {
		return fmt.Errorf("callback function must not be nil")
	}
	ns, err := cnins.GetNS(n.path)
	if err != nil {
		return fmt.Errorf("get netns fd for %q: %w", n.path, err)
	}
	defer ns.Close()
	return ns.Do(f)
}
