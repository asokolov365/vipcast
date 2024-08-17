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

package discovery

import (
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

var nbrs *neighbors
var nbrsLock sync.Mutex

// Storage is the storage backend for all known monitors.
type neighbors struct {
	lock      sync.Mutex
	neighbors map[string]*api.CatalogService
}

// Neighbors returns the storage for all known neighbors.
func Neighbors() *neighbors {
	if nbrs == nil {
		nbrsLock.Lock()
		nbrs = &neighbors{
			lock:      sync.Mutex{},
			neighbors: make(map[string]*api.CatalogService, 100),
		}
		nbrsLock.Unlock()
	}
	return nbrs
}

// Update updates storage in batch mode.
func (n *neighbors) UpdateNeighbors(batch map[string]*api.CatalogService) {
	startTime := time.Now()
	n.lock.Lock()
	defer n.lock.Unlock()

	// Find Neighbors that are not in this batch.
	toRemove := make(map[string]struct{}, len(n.neighbors))
	for addr := range n.neighbors {
		if _, exist := batch[addr]; !exist {
			toRemove[addr] = struct{}{}
		}
	}

	// Remove obsolete Neighbors
	for ip := range toRemove {
		delete(n.neighbors, ip)
	}

	addedCount := 0

	// Add Neighbors that are in this batch
	for addr, nbr := range batch {
		if _, exist := n.neighbors[addr]; !exist {
			addedCount += 1
			logger.Info().
				Str("address", addr).
				Int("port", nbr.ServicePort).
				Msg("adding new neighbor to vipcast")
		}
		n.neighbors[nbr.Address] = nbr
	}

	if addedCount > 0 || len(toRemove) > 0 {
		logger.Info().
			Int("add", addedCount).
			Int("remove", len(toRemove)).
			Int("total", len(n.neighbors)).
			Msgf("spent %d µs in update neighbors", time.Since(startTime).Microseconds())
	}
}
