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
	"github.com/asokolov365/vipcast/registry"
	"github.com/asokolov365/vipcast/route"
)

var reg *MonitorDatabase
var regLock sync.Mutex

// Metrics
var (
	instanceVipsTotal            = metrics.NewGauge(`vipcast_instance_vips_total`, nil)
	instanceVipsHealthy          = metrics.NewGauge(`vipcast_instance_vips_entries{status="healthy"}`, nil)
	instanceVipsNotHealthy       = metrics.NewGauge(`vipcast_instance_vips_entries{status="not-healthy"}`, nil)
	instanceVipsUnderMaintenance = metrics.NewGauge(`vipcast_instance_vips_entries{status="maintenance"}`, nil)
)

// MonitorDatabase is the local (instance wide) Monitor Registry.
type MonitorDatabase struct {
	lock sync.RWMutex
	Data map[string]*Monitor `json:"data,omitempty"`
}

// Registry returns the local Monitor Registry.
func Registry() *MonitorDatabase {
	if reg == nil {
		regLock.Lock()
		reg = &MonitorDatabase{
			lock: sync.RWMutex{},
			Data: make(map[string]*Monitor, 100),
		}
		regLock.Unlock()
	}
	return reg
}

// AddMonitor adds or updates Monitor assosiated with a client VIP.
// See also RemoveMonitor.
//
// Note: This function is not supposed to be used with Consul service discovery.
func (mdb *MonitorDatabase) AddMonitor(newMon *Monitor) {
	if newMon.Registrar() != enum.AdminRegistrar {
		panic("BUG: must be admin registrar")
	}
	startTime := time.Now()

	addedCount := 0

	mdb.lock.Lock()
	defer mdb.lock.Unlock()

	if oldMon, ok := mdb.Data[newMon.VipAddress()]; ok {
		// Don't replace Administratively registered Monitors
		if oldMon.Registrar() == enum.AdminRegistrar {
			logger.Warn().
				Str("service", oldMon.Service()).
				Str("vip", oldMon.VipAddress()).
				Msg("service is already registered to vipcast")
			return
		}
		// Otherwise inherit Health status from the old Monitor
		newMon.SetHealthStatus(oldMon.HealthStatus())

		logger.Debug().
			Str("service", newMon.Service()).
			Str("vip", newMon.VipAddress()).
			Msg("updating service for vipcast")

	} else {
		addedCount++
		logger.Info().
			Str("service", newMon.Service()).
			Str("vip", newMon.VipAddress()).
			Msg("adding new service for vipcast")
		// Setting maintenance off
		// maintenance.MntDB().SetMaintenance(newMon.VipAddress(), false)
	}
	mdb.Data[newMon.VipAddress()] = newMon

	if addedCount > 0 {
		logger.Info().
			Int("add", addedCount).
			Int("remove", 0).
			Int("total", len(mdb.Data)).
			Msgf("spent %d µs updating Monitor Registry", time.Since(startTime).Microseconds())
	}
}

// RemoveMonitor removes Monitor assosiated with a given client VIP.
// See also AddMonitor.
//
// Note: This function is not supposed to be used with Consul service discovery.
func (mdb *MonitorDatabase) RemoveMonitor(vip string) error {
	r, err := route.ParseVIP(vip)
	if err != nil {
		return err
	}
	vip = r.String()

	startTime := time.Now()

	mdb.lock.Lock()
	defer mdb.lock.Unlock()

	if oldMon, ok := mdb.Data[vip]; ok {
		if oldMon.Registrar() == enum.DiscoveryRegistrar {
			return fmt.Errorf("unable to remove %s for Consul service %s", vip, oldMon.Service())
		}
		logger.Info().
			Str("service", oldMon.Service()).
			Str("vip", oldMon.VipAddress()).
			Msg("removing service from vipcast")

		if oldMon.registrar == enum.AdminRegistrar {
			delete(mdb.Data, vip)
		}
		logger.Info().
			Int("add", 0).
			Int("remove", 1).
			Int("total", len(mdb.Data)).
			Msgf("spent %d µs updating Monitor Registry", time.Since(startTime).Microseconds())
		return nil
	}
	return fmt.Errorf("VIP not found: %s", vip)
}

// UpdateDiscoveredMonitors updates Monitor Registry in batch mode.
// See also AddMonitor, RemoveMonitor.
//
// Note: This function is supposed to be used only with Consul service discovery.
func (mdb *MonitorDatabase) UpdateDiscoveredMonitors(batch map[string]*Monitor) {
	startTime := time.Now()

	mdb.lock.Lock()
	defer mdb.lock.Unlock()

	// Find Monitors that are not discovered on this instance.
	// Administratively registered Monitors will not be removed.
	toRemove := make(map[string]struct{}, len(mdb.Data))
	for vip, oldMon := range mdb.Data {
		if _, ok := batch[vip]; !ok && oldMon.Registrar() != enum.AdminRegistrar {
			toRemove[vip] = struct{}{}
		}
	}

	// Remove obsolete Monitors
	for vip := range toRemove {
		delete(mdb.Data, vip)
		registry.Registry().RemoveVipReporter(vip)
	}

	addedCount := 0

	// Add Monitors that are in this batch
	// keeping maintenance and registrar info
	for _, newMon := range batch {
		if oldMon, ok := mdb.Data[newMon.VipAddress()]; ok {
			// Don't replace Administratively registered Monitors
			if oldMon.Registrar() == enum.AdminRegistrar {
				continue
			}
			// Otherwise inherit Health status from the old Monitor
			newMon.SetHealthStatus(oldMon.HealthStatus())

			logger.Debug().
				Str("service", newMon.Service()).
				Str("vip", newMon.VipAddress()).
				Msg("updating service for vipcast")

		} else {
			addedCount++
			registry.Registry().AddVipReporter(newMon.VipAddress())
			if v := registry.Registry().GetVipInfo(newMon.VipAddress()); v == nil {
				err := registry.Registry().SetVipMaintenance(newMon.VipAddress(), false)
				if err != nil {
					logger.Err(err).Send()
				}
			}
			logger.Info().
				Str("service", newMon.Service()).
				Str("vip", newMon.VipAddress()).
				Msg("adding new service for vipcast")
		}
		mdb.Data[newMon.VipAddress()] = newMon
	}

	if addedCount > 0 || len(toRemove) > 0 {
		logger.Info().
			Int("add", addedCount).
			Int("remove", len(toRemove)).
			Int("total", len(mdb.Data)).
			Msgf("spent %d µs updating Monitor Registry", time.Since(startTime).Microseconds())
	}
}

// GetMonitorTargets returns a list of Monitors that should be actively monitored.
//
// withConsul flag indicates whether to return Monitors for Consul health checks.
func (mdb *MonitorDatabase) GetActiveMonitors(withConsul bool) []*Monitor {
	mdb.lock.RLock()
	defer mdb.lock.RUnlock()

	result := make([]*Monitor, 0, len(mdb.Data))
	for _, mon := range mdb.Data {
		// Skipping Monitors that are under maintenance
		if mon.IsVipUnderMaintenance() {
			continue
		}
		// Skipping NoneMonitor Monitors
		if mon.Type() == enum.NoneMonitor {
			continue
		}
		// Skipping Consul Monitors even if we got them somehow registered via API
		if !withConsul && mon.Type() == enum.ConsulMonitor {
			continue
		}
		result = append(result, mon)
	}
	return result
}

// GetHealthyServices returns a list of Monitors that are currently Healthy and
// their VIP is not under Maintenance.
func (mdb *MonitorDatabase) GetHealthyServices() []*Monitor {
	mdb.lock.RLock()
	defer mdb.lock.RUnlock()

	result := make([]*Monitor, 0, len(mdb.Data))
	for _, mon := range mdb.Data {
		// Skipping Monitors that are not healthy
		if mon.HealthStatus() != enum.Healthy || mon.IsVipUnderMaintenance() {
			continue
		}
		result = append(result, mon)
	}
	return result
}

// GetMonitor returns a Monitor for the given VIP.
func (mdb *MonitorDatabase) GetMonitor(vip string) (*Monitor, error) {
	r, err := route.ParseVIP(vip)
	if err != nil {
		return nil, err
	}
	vip = r.String()

	mdb.lock.RLock()
	defer mdb.lock.RUnlock()
	if m, ok := mdb.Data[vip]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("monitor for %s not found", vip)
}

// Len returns the number of Monitors in the Monitor Registry.
func (mdb *MonitorDatabase) Len() int {
	mdb.lock.RLock()
	defer mdb.lock.RUnlock()
	return len(mdb.Data)
}

// UpdateMetrics updates metrics related to the Monitor Registry.
func (mdb *MonitorDatabase) UpdateMetrics() {
	underMaintenanceCount := 0
	healthyCount := 0
	notHealthyCount := 0

	mdb.lock.RLock()
	defer mdb.lock.RUnlock()

	for _, m := range mdb.Data {
		switch {
		case m.IsVipUnderMaintenance():
			underMaintenanceCount++
		case m.HealthStatus() == enum.Healthy:
			healthyCount++
		default:
			notHealthyCount++
		}
	}

	instanceVipsTotal.Set(float64(len(mdb.Data)))
	instanceVipsHealthy.Set(float64(healthyCount))
	instanceVipsNotHealthy.Set(float64(notHealthyCount))
	instanceVipsUnderMaintenance.Set(float64(underMaintenanceCount))
}
