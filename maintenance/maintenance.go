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

package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/asokolov365/vipcast/cluster"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/registry"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var logger *zerolog.Logger

const ApiPath = "/api/v1/maintenance"

var mntDB *MaintDatabase

type VipInfo struct {
	lock               sync.RWMutex
	VipAddress         string `json:"vip"`
	IsUnderMaintenance bool   `json:"maintenance"`
	Generation         int64  `json:"generation"`
}

func (v *VipInfo) setMaintenance(isUnderMaintenance bool) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.IsUnderMaintenance = isUnderMaintenance
	v.Generation = time.Now().UnixMicro()
}

// notifyMaintenance updates the maintenance status of the vip
// only if new Generation is greater than current Generation
func (v *VipInfo) notifyMaintenance(isUnderMaintenance bool, newGeneration int64) bool {
	if newGeneration > v.Generation {
		v.lock.Lock()
		defer v.lock.Unlock()
		v.Generation = newGeneration
		v.IsUnderMaintenance = isUnderMaintenance
		return true
	}
	return false
}

type MaintDatabase struct {
	lock sync.RWMutex
	Data map[string]*VipInfo `json:"data,omitempty"`
}

// MaintDB returns global VIP Maintenance Database.
func MaintDB() *MaintDatabase {
	if mntDB == nil {
		if logger == nil {
			logger = logging.GetSubLogger("maintenance")
		}
		mntDB = &MaintDatabase{
			lock: sync.RWMutex{},
			Data: make(map[string]*VipInfo, 100),
		}
	}
	return mntDB
}

func (m *MaintDatabase) GetVipInfo(vip string) *VipInfo {
	m.lock.Lock()
	defer m.lock.Unlock()
	v, ok := m.Data[vip]
	if !ok {
		return nil
	}
	return v
}

// SetVipMaintenance sets the maintenance flag for the VIP in global VIP Maintenance Database
func (m *MaintDatabase) SetVipMaintenance(vip string, isUnderMaintenance bool) error {
	v := m.GetVipInfo(vip)

	m.lock.Lock()
	defer m.lock.Unlock()

	if v == nil {
		n, err := route.ParseVIP(vip)
		if err != nil {
			return err
		}
		vip = n.String()
		v = &VipInfo{VipAddress: vip, lock: sync.RWMutex{}}
	}
	v.setMaintenance(isUnderMaintenance)
	m.Data[vip] = v
	return nil
}

// NotifyVipMaintenance updates the maintenance status for the vip with newer information.
// It returns true if the maintenance status has been updated, otherwise false.
func (m *MaintDatabase) NotifyVipMaintenance(vip string, isUnderMaintenance bool, newGeneration int64) (bool, error) {
	v := m.GetVipInfo(vip)

	m.lock.Lock()
	defer m.lock.Unlock()

	if v == nil {
		n, err := route.ParseVIP(vip)
		if err != nil {
			return false, err
		}
		vip = n.String()
		v = &VipInfo{VipAddress: vip, lock: sync.RWMutex{}}
	}
	updated := v.notifyMaintenance(isUnderMaintenance, newGeneration)
	if updated {
		m.Data[vip] = v
	}

	return updated, nil
}

func (m *MaintDatabase) AsJSON() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (m *MaintDatabase) Len() int {
	// m.lock.Lock()
	// defer m.lock.Unlock()
	return len(m.Data)
}

const jitterMaxMs uint32 = 1000

func (m *MaintDatabase) BroadcastMaintenance(ctx context.Context) {
	poll := time.Second * 5
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", 5)).
		Msg("starting broadcasting vip maintenance data")

	timeout := time.Second * 2

	for {
		select {
		case <-ticker.C:
			if m.Len() == 0 {
				logger.Debug().Msg("vip maintenance database is empty, nothing to broadcast")
				continue
			}
			jitterMs := fastrand.Uint32n(jitterMaxMs)
			time.Sleep(time.Duration(jitterMs) * time.Millisecond)
			timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
			if body, err := m.AsJSON(); err == nil {
				cluster.VipcastCluster().NotifyCluster(
					timeoutCtx,
					http.MethodPut,
					ApiPath,
					body,
				)
			} else {
				logger.Err(err).Send()
			}
			timeoutCtxCancel()

		case <-ctx.Done():
			logger.Info().Msg("stopping broadcasting vip maintenance data")
			return
		}
	}
}

func (m *MaintDatabase) Cleanup(ctx context.Context, interval int) {
	poll := time.Duration(interval) * time.Second
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", interval)).
		Msg("starting vip maintenance cleanup service")

	for {
		select {
		case <-ticker.C:
			if m.Len() == 0 {
				logger.Debug().Msg("vip maintenance database is empty, nothing to cleanup")
				continue
			}
			startTime := time.Now()
			m.lock.RLock()
			for vip := range m.Data {
				if _, ok := registry.Registry().Data[vip]; !ok {
					logger.Info().Str("vip", vip).
						Msg("removing obsolete vip maintenance data")
					delete(m.Data, vip)
				}
			}
			m.lock.RUnlock()
			logger.Info().Msgf("spent %d µs in vip maintenance data cleanup", time.Since(startTime).Microseconds())

		case <-ctx.Done():
			logger.Info().Msg("stopping vip maintenance cleanup service")
			return
		}
	}
}
