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
	"strings"
	"sync"

	"github.com/asokolov365/vipcast/lib/consul"
)

// ConsulMonitor implements Monitor interface,
// that checks client service health status via Consul API.
type ConsulMonitor struct {
	lock         sync.Mutex
	serviceName  string
	vipInfo      *vipInfo
	maintenance  bool
	healthStatus HealthStatus
}

func NewConsulMonitor(serviceName, vipAddress, bgpCommString string) (*ConsulMonitor, error) {

	vip, err := ParseVIP(vipAddress)
	if err != nil {
		return nil, err
	}

	bgpCommunities, err := ParseBgpCommunities(bgpCommString)
	if err != nil {
		return nil, err
	}

	return &ConsulMonitor{
		vipInfo:      &vipInfo{address: vip, bgpCommunities: bgpCommunities},
		serviceName:  serviceName,
		maintenance:  false,
		healthStatus: Undefined,
	}, nil
}

// IsHealthy implements Monitor interface IsHealthy()
func (m *ConsulMonitor) IsHealthy(ctx context.Context) bool {
	status, err := consul.ApiClient().ServiceHealthStatus(ctx, m.serviceName)
	if err != nil {
		logger.Error().Err(err).
			Str("vip", m.vipInfo.address).
			Str("service", m.serviceName).
			Send()
	}
	return status == "passing"
}

// HealthStatus implements Monitor interface HealthStatus()
func (m *ConsulMonitor) HealthStatus() HealthStatus { return m.healthStatus }

// SetHealthStatus implements Monitor interface SetHealthStatus()
func (m *ConsulMonitor) SetHealthStatus(health HealthStatus) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.healthStatus = health
}

// ServiceName implements Monitor interface ServiceName()
func (m *ConsulMonitor) Service() string { return m.serviceName }

// VipAddress implements Monitor interface VipAddress()
func (m *ConsulMonitor) VipAddress() string { return m.vipInfo.address }

// BgpCommunities implements Monitor interface BgpCommunities()
func (m *ConsulMonitor) BgpCommunities() []string { return m.vipInfo.bgpCommunities }

// SetMaintenance implements Monitor interface SetMaintenance()
func (m *ConsulMonitor) SetMaintenance(isMaintenance bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.maintenance = isMaintenance
}

// IsUnderMaintenance implements Monitor interface IsUnderMaintenance()
func (m *ConsulMonitor) IsUnderMaintenance() bool { return m.maintenance }

// Type implements Monitor interface Type()
func (m *ConsulMonitor) Type() MonitorType {
	return Consul
}

// String implements Monitor interface String()
func (m *ConsulMonitor) String() string {
	str := []string{
		m.Service(),
		m.VipAddress(),
		Consul.String(),
	}
	return strings.Join(str, ":")
}
