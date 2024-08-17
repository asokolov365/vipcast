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
	"fmt"
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/maintenance"
	"github.com/asokolov365/vipcast/route"
)

var storage *monStorage
var storageLock sync.Mutex

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
	monitors map[string]*Monitor
}

// Storage returns the storage for all known monitors.
func Storage() *monStorage {
	if storage == nil {
		storageLock.Lock()
		storage = &monStorage{
			lock:     sync.Mutex{},
			monitors: make(map[string]*Monitor, 100),
		}
		storageLock.Unlock()
	}
	return storage
}

// AddMonitor adds or updates Monitor assosiated with a client VIP.
// See also RemoveMonitor.
//
// Note: This function is not supposed to be used with Consul service discovery.
func (s *monStorage) AddMonitor(newMon *Monitor) {
	if newMon.Registrar() != enum.AdminRegistrar {
		panic("BUG: must be admin registrar")
	}
	startTime := time.Now()
	s.lock.Lock()
	defer s.lock.Unlock()

	addedCount := 0

	if oldMon, exist := s.monitors[newMon.VipAddress()]; exist {
		// Don't replace Administratively registered Monitors
		if oldMon.Registrar() == enum.AdminRegistrar {
			logger.Warn().
				Str("service", oldMon.Service()).
				Str("vip", oldMon.VipAddress()).
				Msg("service is already registered to vipcast")
			return
		}
		// Otherwise inherit the Maintenance and Health status from the old Monitor
		if oldMon.IsUnderMaintenance() {
			newMon.SetMaintenance(enum.MaintenanceOn)
		}
		newMon.SetHealthStatus(oldMon.HealthStatus())

		logger.Debug().
			Str("service", newMon.Service()).
			Str("vip", newMon.VipAddress()).
			Msg("updating service to vipcast")

	} else {
		addedCount++
		logger.Info().
			Str("service", newMon.Service()).
			Str("vip", newMon.VipAddress()).
			Msg("adding new service to vipcast")
		// Setting maintenance off
		maintenance.MntDB().SetMaintenance(newMon.VipAddress(), enum.MaintenanceOff)
	}
	s.monitors[newMon.VipAddress()] = newMon

	if addedCount > 0 {
		logger.Info().
			Int("add", addedCount).
			Int("remove", 0).
			Int("total", len(s.monitors)).
			Msgf("spent %d µs in update client vips", time.Since(startTime).Microseconds())
	}
}

// RemoveMonitor removes Monitor assosiated with a given client VIP.
// See also AddMonitor.
//
// Note: This function is not supposed to be used with Consul service discovery.
func (s *monStorage) RemoveMonitor(vip string) error {
	r, err := route.ParseVIP(vip)
	if err != nil {
		return err
	}
	vip = r.String()

	startTime := time.Now()
	s.lock.Lock()
	defer s.lock.Unlock()
	// If service's been registered with Consul service discovery
	// we can't unregister it, so we set MaintenanceOn for it.
	// Setting maintenance on even if vip does not exist
	maintenance.MntDB().SetMaintenance(vip, enum.MaintenanceOn)

	if oldMon, exist := s.monitors[vip]; exist {
		logger.Info().
			Str("service", oldMon.Service()).
			Str("vip", oldMon.VipAddress()).
			Msg("removing service from vipcast")

		if oldMon.registrar == enum.AdminRegistrar {
			delete(s.monitors, vip)
		}
		logger.Info().
			Int("add", 0).
			Int("remove", 1).
			Int("total", len(s.monitors)).
			Msgf("spent %d µs in update client vips", time.Since(startTime).Microseconds())
		return nil
	}
	return fmt.Errorf("VIP not found: %s", vip)
}

// UpdateDiscoveredMonitors updates storage in batch mode.
// See also AddMonitor, RemoveMonitor.
//
// Note: This function is supposed to be used only with Consul service discovery.
func (s *monStorage) UpdateDiscoveredMonitors(batch map[string]*Monitor) {
	startTime := time.Now()
	s.lock.Lock()
	defer s.lock.Unlock()

	// Find Monitors that are not in this batch.
	// Administratively registered Monitors will not be removed.
	toRemove := make(map[string]struct{}, len(s.monitors))
	for vip, oldMon := range s.monitors {
		if _, ok := batch[vip]; !ok && oldMon.Registrar() != enum.AdminRegistrar {
			toRemove[vip] = struct{}{}
		}
	}

	// Remove obsolete Monitors
	for vip := range toRemove {
		delete(s.monitors, vip)
	}

	addedCount := 0

	// Add Monitors that are in this batch
	// keeping maintenance and registrar info
	for _, newMon := range batch {
		if oldMon, exist := s.monitors[newMon.VipAddress()]; exist {
			// Don't replace Administratively registered Monitors
			if oldMon.Registrar() == enum.AdminRegistrar {
				continue
			}
			// Otherwise inherit the Maintenance and Health status from the old Monitor
			if oldMon.IsUnderMaintenance() {
				newMon.SetMaintenance(enum.MaintenanceOn)
			}
			newMon.SetHealthStatus(oldMon.HealthStatus())

			logger.Debug().
				Str("service", newMon.Service()).
				Str("vip", newMon.VipAddress()).
				Msg("updating service to vipcast")

		} else {
			addedCount++
			logger.Info().
				Str("service", newMon.Service()).
				Str("vip", newMon.VipAddress()).
				Msg("adding new service to vipcast")
		}
		s.monitors[newMon.VipAddress()] = newMon
	}

	if addedCount > 0 || len(toRemove) > 0 {
		logger.Info().
			Int("add", addedCount).
			Int("remove", len(toRemove)).
			Int("total", len(s.monitors)).
			Msgf("spent %d µs in update client vips", time.Since(startTime).Microseconds())
	}
}

func (s *monStorage) GetMonitorTargets(withConsul bool) []*Monitor {
	s.lock.Lock()
	defer s.lock.Unlock()

	result := make([]*Monitor, 0, len(s.monitors))
	for _, mon := range s.monitors {
		// Skipping NoneMonitor Monitors
		if mon.Type() == enum.NoneMonitor {
			continue
		}
		// Skipping Consul Monitors even if we got them somehow registered via API
		if !withConsul && mon.Type() == enum.ConsulMonitor {
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

func (s *monStorage) GetHealthyServices() []*Monitor {
	s.lock.Lock()
	defer s.lock.Unlock()

	result := make([]*Monitor, 0, len(s.monitors))
	for _, mon := range s.monitors {
		// Skipping Monitors that are not healthy
		if mon.HealthStatus() != enum.Healthy || mon.IsUnderMaintenance() {
			continue
		}
		result = append(result, mon)
	}
	return result
}

func (s *monStorage) GetMonitor(vip string) (*Monitor, error) {
	r, err := route.ParseVIP(vip)
	if err != nil {
		return nil, err
	}
	vip = r.String()

	s.lock.Lock()
	defer s.lock.Unlock()
	if m, ok := s.monitors[vip]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("VIP not found: %s", vip)
}

func (s *monStorage) SetMaintenance(vip string, mode enum.MaintenanceMode) error {
	r, err := route.ParseVIP(vip)
	if err != nil {
		return err
	}
	vip = r.String()

	s.lock.Lock()
	defer s.lock.Unlock()
	// Setting maintenance even if vip does not exist
	maintenance.MntDB().SetMaintenance(vip, mode)
	if m, ok := s.monitors[vip]; ok {
		m.SetMaintenance(mode)
		return nil
	}
	return fmt.Errorf("VIP not found: %s", vip)
}

func (s *monStorage) UpdateMetrics() {
	ticker := time.NewTicker(time.Duration(5 * time.Second))
	defer ticker.Stop()

	underMaintenanceCount := 0
	healthyCount := 0
	notHealthyCount := 0

	s.lock.Lock()
	for _, m := range s.monitors {
		switch {
		case m.IsUnderMaintenance():
			underMaintenanceCount++
		case m.HealthStatus() == enum.Healthy:
			healthyCount++
		default:
			notHealthyCount++
		}
	}
	s.lock.Unlock()

	clientVipsEntriesTotal.Set(float64(len(s.monitors)))
	clientVipsEntriesHealthy.Set(float64(healthyCount))
	clientVipsEntriesNotHealthy.Set(float64(notHealthyCount))
	clientVipsEntriesMaintenance.Set(float64(underMaintenanceCount))
}
