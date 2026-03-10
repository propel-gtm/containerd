//go:build linux

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

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestGetImageVolumeSnapshotOptsSmoke(t *testing.T) {
	ctx := context.Background()
	c := &criService{}
	mount := &runtime.Mount{
		ContainerPath: "/smoke",
		UidMappings: []*runtime.IDMapping{{
			ContainerId: 0,
			HostId:      0,
			Length:      1,
		}},
		GidMappings: []*runtime.IDMapping{{
			ContainerId: 0,
			HostId:      0,
			Length:      1,
		}},
	}

	opts, err := c.getImageVolumeSnapshotOpts(ctx, mount)
	require.NoError(t, err)
	assert.NotNil(t, opts)
}
