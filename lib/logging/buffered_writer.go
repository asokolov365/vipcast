// Copyright 2024 Andrew Sokolov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"io"
	"sync"
)

// BufferedWriter is an io.Writer implementation that buffers all of its
// data into an internal buffer until it is told to let data through.
type BufferedWriter struct {
	Writer io.Writer

	buf   [][]byte
	flush bool
	lock  sync.RWMutex
}

// Flush tells the BufferedWriter to flush any buffered data and to stop
// buffering.
func (w *BufferedWriter) Flush() {
	w.lock.Lock()
	w.flush = true
	w.lock.Unlock()

	for _, p := range w.buf {
		w.Write(p)
	}
	w.buf = nil
}

func (w *BufferedWriter) Write(p []byte) (n int, err error) {
	// Once we flush we no longer synchronize writers since there's
	// no use of the internal buffer. This is the happy path.
	w.lock.RLock()
	if w.flush {
		w.lock.RUnlock()
		return w.Writer.Write(p)
	}
	w.lock.RUnlock()

	// Now take the write lock.
	w.lock.Lock()
	defer w.lock.Unlock()

	// Things could have changed between the locking operations, so we
	// have to check one more time.
	if w.flush {
		return w.Writer.Write(p)
	}

	// Buffer up the written data.
	p2 := make([]byte, len(p))
	copy(p2, p)
	w.buf = append(w.buf, p2)
	return len(p), nil
}
