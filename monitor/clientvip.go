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

package monitor

import (
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
)

var (
	clientVIPs = make(map[string]*ClientVIP, 10)
	vipsLock   sync.Mutex
)

// Metrics
var (
	clientVipsEntries = metrics.NewGauge(`vipcast_client_vips_entries`, nil)
)

type ClientVIP struct {
	VipAddress     string
	BgpCommunities []string
	Monitor        *Monitor
	ServiceName    string
	ServiceAddress string
	ServicePort    int
	ServiceTags    []string
}

func AddOne(v *ClientVIP) {
	vipsLock.Lock()
	delete(clientVIPs, v.VipAddress)
	clientVIPs[v.VipAddress] = v
	vipsLock.Unlock()
}

func RemoveOne(v string) {
	vipsLock.Lock()
	delete(clientVIPs, v)
	vipsLock.Unlock()
}

func Update(batch map[string]*ClientVIP) {
	startTime := time.Now()
	vipsLock.Lock()
	defer vipsLock.Unlock()

	// Find clientVIPs that are not in this batch
	clientsToRemove := make(map[string]struct{}, len(clientVIPs))
	for vip := range clientVIPs {
		if _, ok := batch[vip]; !ok {
			clientsToRemove[vip] = struct{}{}
		}
	}

	// Remove obsolete clientVIPs
	for vip := range clientsToRemove {
		delete(clientVIPs, vip)
	}

	// Add clientVIPs that are in this batch
	for _, vipInfo := range batch {
		_, existing := clientVIPs[vipInfo.VipAddress]
		if existing {
			delete(clientVIPs, vipInfo.VipAddress)
		} else {
			logger.Info().
				Str("service", vipInfo.ServiceName).
				Str("vip", vipInfo.VipAddress).
				Strs("tags", vipInfo.ServiceTags).
				Msg("adding new service to vipcast")
		}
		clientVIPs[vipInfo.VipAddress] = vipInfo
	}

	clientVipsEntries.Set(float64(len(clientVIPs)))

	logger.Debug().Msgf("spent %d µs to update client vips", time.Since(startTime).Microseconds())
}

func GetConsulMonitorTargets() map[string]*ClientVIP {
	result := make(map[string]*ClientVIP, len(clientVIPs))

	vipsLock.Lock()
	defer vipsLock.Unlock()

	for vip, vipInfo := range clientVIPs {
		if vipInfo.Monitor.Type == ConsulMonitor {
			result[vip] = vipInfo
		}
	}

	return result
}
