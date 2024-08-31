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

	"github.com/asokolov365/vipcast/cluster"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var logger *zerolog.Logger

const ApiPath = "/api/v1/registry"

var reg *VipDatabase

type VipInfo struct {
	lock       sync.RWMutex
	VipAddress string   `json:"vip"`
	Reporters  []string `json:"reporters"`
	Generation int64    `json:"generation"`
}

func (v *VipInfo) addReporter() {
	reporter := fmt.Sprintf("%s:%d",
		cluster.VipcastCluster().LocalMember().Addr.String(),
		cluster.VipcastCluster().LocalMember().Port,
	)
	v.lock.Lock()
	defer v.lock.Unlock()
	v.Reporters = append(v.Reporters, reporter)
	v.Generation = time.Now().UnixMicro()
}

func (v *VipInfo) removeReporter() {
	reporter := fmt.Sprintf("%s:%d",
		cluster.VipcastCluster().LocalMember().Addr.String(),
		cluster.VipcastCluster().LocalMember().Port,
	)
	v.lock.Lock()
	defer v.lock.Unlock()
	var i int
	var r string
	for i, r = range v.Reporters {
		if r == reporter {
			break
		}
	}
	v.Reporters = append(v.Reporters[:i], v.Reporters[i+1:]...)
	v.Generation = time.Now().UnixMicro()
}

// notifyReporters updates the list of reporters of the vip
// only if new Generation is greater than current Generation
func (v *VipInfo) notifyReporters(reporters []string, newGeneration int64) bool {
	if newGeneration > v.Generation {
		v.lock.Lock()
		defer v.lock.Unlock()
		v.Generation = newGeneration
		v.Reporters = reporters
		return true
	}
	return false
}

type VipDatabase struct {
	lock sync.RWMutex
	Data map[string]*VipInfo `json:"data,omitempty"`
}

// Registry returns global VIP Registry.
func Registry() *VipDatabase {
	if reg == nil {
		if logger == nil {
			logger = logging.GetSubLogger("registry")
		}
		reg = &VipDatabase{
			lock: sync.RWMutex{},
			Data: make(map[string]*VipInfo, 100),
		}
	}
	return reg
}

func (vdb *VipDatabase) GetVipInfo(vip string) *VipInfo {
	vdb.lock.Lock()
	defer vdb.lock.Unlock()
	v, ok := vdb.Data[vip]
	if !ok {
		return nil
	}
	return v
}

// AddVipReporter adds the VIP reporter in global VIP Registry.
//
// If VIP does not exist it register a new VIP.
func (vdb *VipDatabase) AddVipReporter(vip string) error {
	v := vdb.GetVipInfo(vip)

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	if v == nil {
		n, err := route.ParseVIP(vip)
		if err != nil {
			return err
		}
		vip = n.String()
		v = &VipInfo{VipAddress: vip, lock: sync.RWMutex{}}
	}
	v.addReporter()
	vdb.Data[vip] = v
	return nil
}

// RemoveVipReporter removes the VIP reporter in global VIP Registry.
//
// If VIP does not have any reporters the function also removes the VIP from the registry.
func (vdb *VipDatabase) RemoveVipReporter(vip string) {
	v := vdb.GetVipInfo(vip)
	if v == nil {
		return
	}

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	v.removeReporter()
	if len(v.Reporters) == 0 {
		delete(vdb.Data, vip)
		return
	}
	vdb.Data[vip] = v
}

// NotifyVipReporters updates the list of reporters for the vip with newer information.
// It returns true if the list of reporters has been updated, otherwise false.
func (vdb *VipDatabase) NotifyVipReporters(vip string, reporters []string, newGeneration int64) (bool, error) {
	v := vdb.GetVipInfo(vip)

	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	if v == nil {
		n, err := route.ParseVIP(vip)
		if err != nil {
			return false, err
		}
		vip = n.String()
		v = &VipInfo{VipAddress: vip, lock: sync.RWMutex{}}
	}
	updated := v.notifyReporters(reporters, newGeneration)
	if updated {
		vdb.Data[vip] = v
	}

	return updated, nil
}

func (vdb *VipDatabase) AsJSON() ([]byte, error) {
	vdb.lock.Lock()
	defer vdb.lock.Unlock()

	b, err := json.MarshalIndent(vdb, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (vdb *VipDatabase) Len() int {
	// m.lock.Lock()
	// defer m.lock.Unlock()
	return len(vdb.Data)
}

const jitterMaxMs uint32 = 1000

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
			logger.Info().Msg("stopping broadcasting vip registry")
			return
		}
	}
}
