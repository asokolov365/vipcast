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

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/cluster"
	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var logger *zerolog.Logger

var reg *VipDatabase
var regLock sync.Mutex

// Metrics
var (
	clusterVipsTotal            = metrics.NewGauge(`vipcast_cluster_vips_total`, nil)
	clusterVipsUnderMaintenance = metrics.NewGauge(`vipcast_cluster_vips_entries{status="maintenance"}`, nil)
)

// Maintenance is a struct that holds the VIP maintenance status.
type Maintenance struct {
	lock               sync.Mutex
	IsUnderMaintenance bool  `json:"enabled"`
	Generation         int64 `json:"generation"`
}

// Reporters is a struct that holds the list of cluster members
// that are advertising the VIP.
type Reporters struct {
	lock       sync.Mutex
	Members    []string `json:"members"`
	Generation int64    `json:"generation"`
}

// VipInfo is a struct that holds information about the VIP.
type VipInfo struct {
	VipAddress  string      `json:"vip"`
	Reporters   Reporters   `json:"reporters"`
	Maintenance Maintenance `json:"maintenance"`
}

func (v *VipInfo) addReporter() {
	reporter := fmt.Sprintf("%s:%d",
		cluster.VipcastCluster().LocalMember().Addr.String(),
		cluster.VipcastCluster().LocalMember().Port,
	)
	v.Reporters.lock.Lock()
	defer v.Reporters.lock.Unlock()
	found := false
	for _, r := range v.Reporters.Members {
		if r == reporter {
			found = true
			break
		}
	}
	if !found {
		v.Reporters.Members = append(v.Reporters.Members, reporter)
		v.Reporters.Generation = time.Now().UnixMicro()
	}
}

func (v *VipInfo) removeReporter() {
	if v.Reporters.Members == nil || len(v.Reporters.Members) == 0 {
		return
	}
	reporter := fmt.Sprintf("%s:%d",
		cluster.VipcastCluster().LocalMember().Addr.String(),
		cluster.VipcastCluster().LocalMember().Port,
	)
	v.Reporters.lock.Lock()
	defer v.Reporters.lock.Unlock()
	var i int
	var m string
	for i, m = range v.Reporters.Members {
		if m == reporter {
			v.Reporters.Members = append(v.Reporters.Members[:i], v.Reporters.Members[i+1:]...)
			v.Reporters.Generation = time.Now().UnixMicro()
			break
		}
	}
}

// notifyReporters updates current VIP reporters list with newer information,
// but only if new Generation is greater than the current Generation.
func (v *VipInfo) notifyReporters(members []string, newGeneration int64) bool {
	if newGeneration > v.Reporters.Generation {
		v.Reporters.lock.Lock()
		defer v.Reporters.lock.Unlock()
		v.Reporters.Generation = newGeneration
		v.Reporters.Members = members
		return true
	}
	return false
}

func (v *VipInfo) setMaintenance(isUnderMaintenance bool) {
	v.Maintenance.lock.Lock()
	defer v.Maintenance.lock.Unlock()
	v.Maintenance.IsUnderMaintenance = isUnderMaintenance
	v.Maintenance.Generation = time.Now().UnixMicro()
}

// notifyMaintenance updates current VIP maintenance status with newer information,
// but only if new Generation is greater than the current Generation.
func (v *VipInfo) notifyMaintenance(isUnderMaintenance bool, newGeneration int64) bool {
	if newGeneration > v.Maintenance.Generation {
		v.Maintenance.lock.Lock()
		defer v.Maintenance.lock.Unlock()
		v.Maintenance.Generation = newGeneration
		v.Maintenance.IsUnderMaintenance = isUnderMaintenance
		return true
	}
	return false
}

func (v *VipInfo) IsUnderMaintenance() bool {
	return v.Maintenance.IsUnderMaintenance
}

// VipDatabase is the global (cluster wide) VIP Registry.
type VipDatabase struct {
	lock sync.RWMutex
	Data map[string]*VipInfo `json:"data,omitempty"`
}

// Registry returns global VIP Registry Database.
func Registry() *VipDatabase {
	if reg == nil {
		if logger == nil {
			logger = logging.GetSubLogger("registry")
		}
		regLock.Lock()
		reg = &VipDatabase{
			lock: sync.RWMutex{},
			Data: make(map[string]*VipInfo, 100),
		}
		regLock.Unlock()
	}
	return reg
}

// GetVipInfo returns VipInfo struct associated with the given VIP.
func (vdb *VipDatabase) GetVipInfo(vip string) *VipInfo {
	vdb.lock.RLock()
	defer vdb.lock.RUnlock()
	if v, ok := vdb.Data[vip]; ok {
		return v
	}
	return nil
}

// AddVipReporter adds this vipcast cluster member Addr:Port as a reporter for the given VIP.
//
// If VIP does not exist it registers a new VIP.
func (vdb *VipDatabase) AddVipReporter(vip string) error {
	n, err := route.ParseVIP(vip)
	if err != nil {
		return err
	}

	defer vdb.UpdateMetrics()

	vip = n.String()
	v := vdb.GetVipInfo(vip)

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	if v == nil {
		v = &VipInfo{
			VipAddress: vip,
			Reporters:  Reporters{Members: []string{}, lock: sync.Mutex{}},
		}
	}
	v.addReporter()
	vdb.Data[vip] = v
	return nil
}

// RemoveVipReporter removes this vipcast cluster member Addr:Port as a reporter for the given VIP.
//
// If VIP does not have any reporters RemoveVipReporter also removes the VIP from the global VIP Registry.
func (vdb *VipDatabase) RemoveVipReporter(vip string) {
	v := vdb.GetVipInfo(vip)
	if v == nil {
		return
	}

	defer vdb.UpdateMetrics()

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	v.removeReporter()
	if len(v.Reporters.Members) == 0 {
		delete(vdb.Data, vip)
		return
	}
	vdb.Data[vip] = v
}

// SetVipMaintenance sets the maintenance flag for the given VIP in the global VIP Registry.
func (vdb *VipDatabase) SetVipMaintenance(vip string, isUnderMaintenance bool) error {
	n, err := route.ParseVIP(vip)
	if err != nil {
		return err
	}

	defer vdb.UpdateMetrics()

	vip = n.String()
	v := vdb.GetVipInfo(vip)

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	if v == nil {
		v = &VipInfo{
			VipAddress: vip,
			Reporters:  Reporters{Members: []string{}, lock: sync.Mutex{}},
		}
	}
	v.setMaintenance(isUnderMaintenance)
	vdb.Data[vip] = v
	return nil
}

// NotifyVipInfoChange attempts to update current VIP properties with newer information.
//
// It returns true if any property has been changed, otherwise false.
func (vdb *VipDatabase) NotifyVipInfoChange(nvi *VipInfo) (bool, error) {
	if nvi == nil {
		panic("BUG: got nil arg")
	}
	n, err := route.ParseVIP(nvi.VipAddress)
	if err != nil {
		return false, err
	}

	defer vdb.UpdateMetrics()

	vip := n.String()
	v := vdb.GetVipInfo(vip)

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	if v == nil {
		v = &VipInfo{
			VipAddress: vip,
			Reporters:  Reporters{Members: []string{}, lock: sync.Mutex{}},
		}
	}

	repsUpdated := v.notifyReporters(nvi.Reporters.Members, nvi.Reporters.Generation)
	mntUpdated := v.notifyMaintenance(nvi.Maintenance.IsUnderMaintenance, nvi.Maintenance.Generation)
	if repsUpdated || mntUpdated {
		vdb.Data[vip] = v
	}

	return repsUpdated || mntUpdated, nil
}

// AsJSON returns a JSON representation of the global VIP Registry.
func (vdb *VipDatabase) AsJSON() ([]byte, error) {
	vdb.lock.RLock()
	defer vdb.lock.RUnlock()

	b, err := json.MarshalIndent(vdb, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Len returns the number of VIPs in the global VIP Registry.
func (vdb *VipDatabase) Len() int {
	vdb.lock.RLock()
	defer vdb.lock.RUnlock()
	return len(vdb.Data)
}

const jitterMaxMs uint32 = 1000

// BroadcastRegistry broadcasts the global VIP Registry to other members of the vipcast cluster.
func (vdb *VipDatabase) BroadcastRegistry(ctx context.Context) {
	poll := time.Second * 5
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", 5)).
		Msg("starting broadcasting vip registry")

	timeout := time.Second * 2

	for {
		select {
		case <-ticker.C:
			if vdb.Len() == 0 {
				logger.Debug().Msg("vip registry is empty, nothing to broadcast")
				continue
			}
			jitterMs := fastrand.Uint32n(jitterMaxMs)
			time.Sleep(time.Duration(jitterMs) * time.Millisecond)
			timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
			if body, err := vdb.AsJSON(); err == nil {
				cluster.VipcastCluster().NotifyCluster(
					timeoutCtx, http.MethodPut, enum.RegistryApiPath, body,
				)
			} else {
				logger.Err(err).Send()
			}
			timeoutCtxCancel()

		case <-ctx.Done():
			logger.Info().Msg("stopping broadcasting vip registry")
			return
		}
	}
}

// Cleanup removes obsolete VIPs from the global VIP Registry.
//
// VIP is obsolete when it's not reported by any member of the vipcast cluster.
func (vdb *VipDatabase) Cleanup(ctx context.Context, interval int) {
	poll := time.Duration(interval) * time.Second
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", interval)).
		Msg("starting vip registry cleanup service")

	var toRemove []string

	for {
		select {
		case <-ticker.C:
			if vdb.Len() == 0 {
				logger.Debug().Msg("vip registry is empty, nothing to cleanup")
				continue
			}
			startTime := time.Now()
			vdb.lock.Lock()
			// Cleaning up previous toRemove batch
			for _, vip := range toRemove {
				if v, ok := vdb.Data[vip]; ok {
					if len(v.Reporters.Members) == 0 {
						logger.Info().Str("vip", vip).Msg("removing obsolete vip")
						delete(vdb.Data, vip)
					}
				}
			}
			vdb.lock.Unlock()
			vdb.UpdateMetrics()

			// Populating toRemove batch for the next time
			toRemove = make([]string, 0, vdb.Len())

			vdb.lock.RLock()
			for vip, v := range vdb.Data {
				if len(v.Reporters.Members) == 0 {
					logger.Info().Str("vip", vip).Msgf("vip is obsolete and will be removed from registry in %.fs", poll.Seconds())
					toRemove = append(toRemove, vip)
				}
			}
			vdb.lock.RUnlock()

			logger.Info().Msgf("spent %d µs in vip registry cleanup", time.Since(startTime).Microseconds())

		case <-ctx.Done():
			logger.Info().Msg("stopping vip registry cleanup service")
			return
		}
	}
}

// UpdateMetrics updates metrics related to the global VIP Registry.
func (vdb *VipDatabase) UpdateMetrics() {
	underMaintenanceCount := 0

	vdb.lock.RLock()
	defer vdb.lock.RUnlock()

	for _, v := range vdb.Data {
		if v.Maintenance.IsUnderMaintenance {
			underMaintenanceCount++
		}
	}

	clusterVipsTotal.Set(float64(len(vdb.Data)))
	clusterVipsUnderMaintenance.Set(float64(underMaintenanceCount))
}
