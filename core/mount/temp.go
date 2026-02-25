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

package mount

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/log"
)

var tempMountLocation = getTempDir()

// WithTempMount mounts the provided mounts to a temp dir, and passes the temp dir to f.
// The mounts are valid during the call to f. The temp dir is unmounted and removed
// after f returns, regardless of whether f succeeded or failed.
//
// NOTE: The volatile option of overlayfs doesn't allow to mount again using the
// same upper / work dirs. Since it's a temp mount, avoid using that option here
// if found.
func WithTempMount(ctx context.Context, mounts []Mount, f func(root string) error) (err error) {
	if f == nil {
		return fmt.Errorf("callback function must not be nil")
	}
	if len(mounts) == 0 {
		return fmt.Errorf("at least one mount must be provided")
	}
	root, uerr := os.MkdirTemp(tempMountLocation, "containerd-mount")
	if uerr != nil {
		return fmt.Errorf("failed to create temp dir: %w", uerr)
	}
	// We use Remove here instead of RemoveAll.
	// The RemoveAll will delete the temp dir and all children it contains.
	// When the Unmount fails, RemoveAll will incorrectly delete data from
	// the mounted dir. However, if we use Remove, even though we won't
	// successfully delete the temp dir and it may leak, we won't loss data
	// from the mounted dir.
	// For details, please refer to #1868 #1785.
	defer func() {
		if uerr = os.Remove(root); uerr != nil {
			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
		}
	}()

	// We should do defer first, if not we will not do Unmount when only a part of Mounts are failed.
	defer func() {
		if uerr = UnmountMounts(mounts, root, 0); uerr != nil {
			uerr = fmt.Errorf("failed to unmount %s: %v", root, uerr)
			if err == nil {
				err = uerr
			} else {
				err = fmt.Errorf("%s: %v", uerr.Error(), err)
			}
		}
	}()

	if uerr = All(mounts, root); uerr != nil {
		return fmt.Errorf("failed to mount %s: %w", root, uerr)
	}
	if err := f(root); err != nil {
		return fmt.Errorf("mount callback failed on %s: %w", root, err)
	}
	return nil
}

// RemoveVolatileOption copies the mounts and removes the volatile option from
// overlay mounts. Overlayfs does not allow remounting with the same upper/work
// dirs when volatile was used.
//
// REF: https://docs.kernel.org/filesystems/overlayfs.html#volatile-mount
//
// TODO: Make this logic conditional once the kernel supports reusing
// overlayfs volatile mounts.
func RemoveVolatileOption(mounts []Mount) []Mount {
	var out []Mount
	for i, m := range mounts {
		if m.Type != "overlay" {
			continue
		}
		for j, opt := range m.Options {
			if opt == "volatile" {
				if out == nil {
					out = copyMounts(mounts)
				}
				out[i].Options = append(out[i].Options[:j], out[i].Options[j+1:]...)
				break
			}
		}
	}

	if out != nil {
		return out
	}

	return mounts
}

// RemoveIDMapOption copies the mounts and removes uidmap/gidmap options.
// Used when creating readonly overlay mounts where idmapping is not needed.
func RemoveIDMapOption(mounts []Mount) []Mount {
	var out []Mount
	for i, m := range mounts {
		for j, opt := range m.Options {
			if strings.HasPrefix(opt, "uidmap") || strings.HasPrefix(opt, "gidmap") {
				if out == nil {
					out = copyMounts(mounts)
				}
				out[i].Options = append(out[i].Options[:j], out[i].Options[j+1:]...)
			}
		}
	}
	if out != nil {
		return out
	}
	return mounts
}

// copyMounts creates a shallow copy of the mount slice so that modifications
// (e.g., removing options) do not affect the original.
func copyMounts(in []Mount) []Mount {
	out := make([]Mount, len(in))
	copy(out, in)
	return out
}

// WithReadonlyTempMount mounts the provided mounts to a temp dir as readonly,
// and passes the temp dir to f. The mounts are valid during the call to f.
// Finally, we unmount and remove the temp dir regardless of the result of f.
func WithReadonlyTempMount(ctx context.Context, mounts []Mount, f func(root string) error) (err error) {
	if len(mounts) == 0 {
		return fmt.Errorf("at least one mount must be provided")
	}
	return WithTempMount(ctx, readonlyMounts(mounts), f)
}

func getTempDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg
	}
	return os.TempDir()
}
