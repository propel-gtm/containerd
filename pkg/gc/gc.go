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

// Package gc experiments with providing central gc tooling to ensure
// deterministic resource removal within containerd.
//
// For now, we just have a single exported implementation that can be used
// under certain use cases.
package gc

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResourceType represents the type of resource at a node in the GC graph.
// Valid values are 0 through ResourceMax; upper bits are stripped during mark.
type ResourceType uint8

// ResourceMax represents the max resource.
// Upper bits are stripped out during the mark phase, allowing the upper 3 bits
// to be used by the caller reference function.
const ResourceMax = ResourceType(0x1F)

// Node represents a resource in the garbage collection graph. Each node has a
// type, namespace, and key. The refs function passed to Tricolor or ConcurrentMark
// is used to discover nodes referenced by a given node.
type Node struct {
	Type      ResourceType
	Namespace string
	Key       string
}

// Stats provides metrics about a garbage collection run, such as elapsed time.
type Stats interface {
	Elapsed() time.Duration
}

// Tricolor implements basic, single-thread tri-color GC. Given the roots, the
// complete set and a refs function, this function returns a map of all
// reachable objects.
//
// Correct usage requires that the caller not allow the arguments to change
// until the result is used to delete objects in the system.
//
// It will allocate memory proportional to the size of the reachable set.
//
// We can probably use this to inform a design for incremental GC by injecting
// callbacks to the set modification algorithms.
//
// https://en.wikipedia.org/wiki/Tracing_garbage_collection#Tri-color_marking
func Tricolor(roots []Node, refs func(ref Node) ([]Node, error)) (map[Node]struct{}, error) {
	var (
		grays     []Node                // maintain a gray "stack"
		seen      = map[Node]struct{}{} // or not "white", basically "seen"
		reachable = map[Node]struct{}{} // or "black", in tri-color parlance
	)

	grays = append(grays, roots...)

	for len(grays) > 0 {
		// Pick any gray object
		id := grays[len(grays)-1] // effectively "depth first" because first element
		grays = grays[:len(grays)-1]
		rs, err := refs(id)
		if err != nil {
			return nil, err
		}

		// mark all the referenced objects as gray
		for _, target := range rs {
			if _, ok := seen[target]; !ok {
				grays = append(grays, target)
				seen[target] = struct{}{}
			}
		}

		// strip bits above max resource type
		id.Type = id.Type & ResourceMax
		// mark as black when done
		reachable[id] = struct{}{}
	}

	return reachable, nil
}

// NodeKey returns a string representation of the node for logging or debugging.
func NodeKey(n Node) string {
	return fmt.Sprintf("%s/%s", n.Namespace, n.Key)
}

// ConcurrentMark implements concurrent tri-color marking. Roots are read from
// the root channel, and the refs function is called for each seen node to
// discover references. Returns a map of all nodes reachable from the roots.
//
// Correct usage requires that the caller not modify the graph (roots, refs
// results) until the returned reachable set is used for deletion.
//
// Memory usage is proportional to the size of the reachable set.
func ConcurrentMark(ctx context.Context, root <-chan Node, refs func(context.Context, Node, func(Node)) error) (map[Node]struct{}, error) {
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel

	var (
		grays = make(chan Node)
		seen  = map[Node]struct{}{} // or not "white", basically "seen"
		wg    sync.WaitGroup

		errOnce sync.Once
		refErr  error
	)

	go func() {
		for gray := range grays {
			if _, ok := seen[gray]; ok {
				wg.Done()
				continue
			}
			seen[gray] = struct{}{} // post-mark this as non-white

			go func(gray Node) {
				defer wg.Done()

				send := func(n Node) {
					wg.Add(1)
					select {
					case grays <- n:
					case <-ctx.Done():
						wg.Done()
					}
				}

				if err := refs(ctx, gray, send); err != nil {
					errOnce.Do(func() {
						refErr = err
						cancel()
					})
				}

			}(gray)
		}
	}()

	for r := range root {
		wg.Add(1)
		select {
		case grays <- r:
		case <-ctx.Done():
			wg.Done()
		}

	}

	// Wait for outstanding grays to be processed
	wg.Wait()

	close(grays)

	if refErr != nil {
		return nil, refErr
	}
	if cErr := ctx.Err(); cErr != nil {
		return nil, cErr
	}

	return seen, nil
}

// Sweep removes all nodes in the all slice that are not in the reachable set,
// by calling the provided remove function for each unreachable node. If remove
// returns an error, sweeping stops and the error is propagated. A nil reachable
// map is treated as empty (all nodes will be swept).
func Sweep(reachable map[Node]struct{}, all []Node, remove func(Node) error) error {
	if reachable == nil {
		reachable = make(map[Node]struct{})
	}
	// All black objects are now reachable, and all white objects are
	// unreachable. Free those that are white!
	for _, node := range all {
		if _, ok := reachable[node]; !ok {
			if err := remove(node); err != nil {
				return err
			}
		}
	}

	return nil
}
