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
	"context"
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
)

// Metrics
var (
	clientVipsEntriesTotal       = metrics.NewGauge(`vipcast_client_vips_all_total`, nil)
	clientVipsEntriesHealthy     = metrics.NewGauge(`vipcast_client_vips_total{status="healthy"}`, nil)
	clientVipsEntriesNotHealthy  = metrics.NewGauge(`vipcast_client_vips_total{status="not-healthy"}`, nil)
	clientVipsEntriesMaintenance = metrics.NewGauge(`vipcast_client_vips_total{status="maintenance"}`, nil)
)

// Storage is the storage backend for all known monitors.
type monStorage struct {
	lock     sync.Mutex
	monitors map[string]Monitor
}

func NewStorage() *monStorage {
	return &monStorage{
		lock:     sync.Mutex{},
		monitors: make(map[string]Monitor, 100),
	}
}

func (s *monStorage) AddOne(m Monitor) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// delete(s.monitors, m.VipAddress())
	s.monitors[m.VipAddress()] = m
}

func (s *monStorage) RemoveOne(vip string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.monitors, vip)
}

func (s *monStorage) Update(batch map[string]Monitor) {
	startTime := time.Now()
	s.lock.Lock()
	defer s.lock.Unlock()

	// Find Monitors that are not in this batch
	toRemove := make(map[string]struct{}, len(s.monitors))
	for vip := range s.monitors {
		if _, ok := batch[vip]; !ok {
			toRemove[vip] = struct{}{}
		}
	}

	// Remove obsolete Monitors
	for vip := range toRemove {
		delete(s.monitors, vip)
	}

	addedCount := 0

	// Add Monitors that are in this batch keeping maintenance status
	for _, newMon := range batch {
		oldMon, existing := s.monitors[newMon.VipAddress()]
		if existing {
			newMon.SetMaintenance(oldMon.IsUnderMaintenance())

		} else {
			addedCount += 1
			logger.Info().
				Str("service", newMon.Service()).
				Str("vip", newMon.VipAddress()).
				Msg("adding new service to vipcast")
		}
		s.monitors[newMon.VipAddress()] = newMon
	}

	logger.Info().
		Int("add", addedCount).
		Int("remove", len(toRemove)).
		Int("total", len(s.monitors)).
		Msgf("spent %d µs in update client vips", time.Since(startTime).Microseconds())
}

func (s *monStorage) GetMonitorTargets(withConsul bool) []Monitor {
	s.lock.Lock()
	defer s.lock.Unlock()

	result := make([]Monitor, 0, len(s.monitors))
	for _, mon := range s.monitors {
		// Skipping NoneMonitor Monitors
		if mon.Type() == None {
			continue
		}
		// Skipping Consul Monitors even if we got them somehow registered via API
		if !withConsul && mon.Type() == Consul {
			continue
		}
		// Skipping Monitors that are under maintenance
		if mon.IsUnderMaintenance() {
			continue
		}
		result = append(result, mon)
	}
	return result
}

func (s *monStorage) GetHealthyServices() []Monitor {
	s.lock.Lock()
	defer s.lock.Unlock()

	result := make([]Monitor, 0, len(s.monitors))
	for _, mon := range s.monitors {
		// Skipping Monitors that are not healthy
		if mon.HealthStatus() != Healthy || mon.IsUnderMaintenance() {
			continue
		}
		result = append(result, mon)
	}
	return result
}

func (s *monStorage) UpdateMetrics(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(5 * time.Second))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			underMaintenanceCount := 0
			healthyCount := 0
			notHealthyCount := 0
			for _, m := range s.monitors {
				switch {
				case m.IsUnderMaintenance():
					underMaintenanceCount++
				case m.HealthStatus() == Healthy:
					healthyCount++
				default:
					notHealthyCount++
				}
			}
			clientVipsEntriesTotal.Set(float64(len(s.monitors)))
			clientVipsEntriesHealthy.Set(float64(healthyCount))
			clientVipsEntriesNotHealthy.Set(float64(notHealthyCount))
			clientVipsEntriesMaintenance.Set(float64(underMaintenanceCount))

		case <-ctx.Done():
			return
		}
	}
}
