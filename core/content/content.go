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

package content

import (
	"context"
	"io"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Store combines the methods of content-oriented interfaces into a set that
// are commonly provided by complete implementations.
//
// Overall content lifecycle:
//   - Ingester is used to initiate a write operation (aka ingestion)
//   - IngestManager is used to manage (e.g. list, abort) active ingestions
//   - Once an ingestion is complete (see Writer.Commit), Provider is used to
//     query a single piece of content by its digest
//   - Manager is used to manage (e.g. list, delete) previously committed content
//
// Note that until ingestion is complete, its content is not visible through
// Provider or Manager. Once ingestion is complete, it is no longer exposed
// through IngestManager.
type Store interface {
	Manager
	Provider
	IngestManager
	Ingester
}

// ReaderAt extends io.ReaderAt with Size and Close. Used by Provider to
// expose content for reading. Callers must call Close when done.
type ReaderAt interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

// Provider provides a reader for content by digest. Only desc.Digest is
// required; other descriptor fields may be used internally.
type Provider interface {
	// ReaderAt only requires desc.Digest to be set.
	// Other fields in the descriptor may be used internally for resolving
	// the location of the actual data.
	ReaderAt(ctx context.Context, desc ocispec.Descriptor) (ReaderAt, error)
}

// Ingester initiates writes (ingestions). Each ingestion is identified by
// a ref; use WithRef when calling Writer.
type Ingester interface {
	// Writer initiates a writing operation (aka ingestion). A single ingestion
	// is uniquely identified by its ref, provided using a WithRef option.
	// Writer can be called multiple times with the same ref to access the same
	// ingestion.
	// Once all the data is written, use Writer.Commit to complete the ingestion.
	Writer(ctx context.Context, opts ...WriterOpt) (Writer, error)
}

// IngestManager manages active ingestions (writes in progress). Ingestions
// are identified by the ref string passed to Writer.
type IngestManager interface {
	// Status returns the status of the provided ref.
	Status(ctx context.Context, ref string) (Status, error)

	// ListStatuses returns the status of any active ingestions whose ref match
	// the provided regular expression. If empty, all active ingestions will be
	// returned.
	ListStatuses(ctx context.Context, filters ...string) ([]Status, error)

	// Abort completely cancels the ingest operation targeted by ref.
	Abort(ctx context.Context, ref string) error
}

// Info holds content specific information
type Info struct {
	Digest    digest.Digest
	Size      int64
	CreatedAt time.Time
	UpdatedAt time.Time
	Labels    map[string]string
}

// Status of a content operation (i.e. an ingestion)
type Status struct {
	Ref       string
	Offset    int64
	Total     int64
	Expected  digest.Digest
	StartedAt time.Time
	UpdatedAt time.Time
}

// WalkFunc defines the callback for a blob walk.
type WalkFunc func(Info) error

// InfoReaderProvider provides both info and reader for the specific content.
type InfoReaderProvider interface {
	InfoProvider
	Provider
}

// InfoProvider provides info for content inspection.
type InfoProvider interface {
	// Info will return metadata about content available in the content store.
	//
	// If the content is not present, ErrNotFound will be returned.
	Info(ctx context.Context, dgst digest.Digest) (Info, error)
}

// Manager provides methods for inspecting, listing and removing content.
type Manager interface {
	InfoProvider

	// Update updates mutable information related to content.
	// If one or more fieldpaths are provided, only those
	// fields will be updated.
	// Mutable fields:
	//  labels.*
	Update(ctx context.Context, info Info, fieldpaths ...string) (Info, error)

	// Walk will call fn for each item in the content store which
	// match the provided filters. If no filters are given all
	// items will be walked.
	Walk(ctx context.Context, fn WalkFunc, filters ...string) error

	// Delete removes the content from the store.
	Delete(ctx context.Context, dgst digest.Digest) error
}

// Writer handles writing of content into a content store
type Writer interface {
	// Close closes the writer, if the writer has not been
	// committed this allows resuming or aborting.
	// Calling Close on a closed writer will not error.
	io.WriteCloser

	// Digest may return empty digest or panics until committed.
	Digest() digest.Digest

	// Commit commits the blob (but no roll-back is guaranteed on an error).
	// size and expected can be zero-value when unknown.
	// Commit always closes the writer, even on error.
	// ErrAlreadyExists aborts the writer.
	Commit(ctx context.Context, size int64, expected digest.Digest, opts ...Opt) error

	// Status returns the current state of write
	Status() (Status, error)

	// Truncate updates the size of the target blob
	Truncate(size int64) error
}

type Syncer interface {
	// Sync flushes the in-flight writes to the disk (when applicable)
	Sync() error
}

// ReferrersProvider handles looking up additional referrer objects for a given descriptor.
type ReferrersProvider interface {
	Referrers(context.Context, ocispec.Descriptor) ([]ocispec.Descriptor, error)
}

// Opt is used to alter the mutable properties of content
type Opt func(*Info) error

// WithLabels allows labels to be set on content
func WithLabels(labels map[string]string) Opt {
	return func(info *Info) error {
		info.Labels = labels
		return nil
	}
}

// WriterOpts is internally used by WriterOpt.
type WriterOpts struct {
	Ref  string
	Desc ocispec.Descriptor
}

// WriterOpt is used for passing options to Ingester.Writer.
type WriterOpt func(*WriterOpts) error

// WithDescriptor specifies an OCI descriptor.
// Writer may optionally use the descriptor internally for resolving
// the location of the actual data.
// Write does not require any field of desc to be set.
// If the data size is unknown, desc.Size should be set to 0.
// Some implementations may also accept negative values as "unknown".
func WithDescriptor(desc ocispec.Descriptor) WriterOpt {
	return func(opts *WriterOpts) error {
		opts.Desc = desc
		return nil
	}
}

// WithRef specifies a ref string.
func WithRef(ref string) WriterOpt {
	return func(opts *WriterOpts) error {
		opts.Ref = ref
		return nil
	}
}
