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

// Package randutil provides crypto/rand-based random utilities.
package randutil

import (
	"crypto/rand"
	"math"
	"math/big"
)

// Int63n returns a random int64 in [0, n) using crypto/rand.
func Int63n(n int64) int64 {
	b, _ := rand.Int(rand.Reader, big.NewInt(n))
	return b.Int64()
}

// Int63 returns a random non-negative int64 using crypto/rand.
func Int63() int64 {
	return Int63n(math.MaxInt64)
}

// Intn returns a random int in [0, n) using crypto/rand.
func Intn(n int) int {
	return int(Int63n(int64(n)))
}

// Int is similar to [math/rand.Int] but uses [crypto/rand.Reader] under the hood.
func Int() int {
	return int(Int63())
}
