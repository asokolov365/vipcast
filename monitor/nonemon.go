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
)

// NoneMonitor implements Monitor interface,
// that doesn't checks client service health status.
type NoneMonitor struct {
	serviceName  string
	vipInfo      *vipInfo
	maintenance  bool
	healthStatus HealthStatus
}

func NewNoneMonitor(serviceName, vipAddress, bgpCommString string) (*NoneMonitor, error) {
	vip, err := ParseVIP(vipAddress)
	if err != nil {
		return nil, err
	}
	bgpCommunities, err := ParseBgpCommunities(bgpCommString)
	if err != nil {
		return nil, err
	}
	return &NoneMonitor{
		vipInfo:      &vipInfo{address: vip, bgpCommunities: bgpCommunities},
		serviceName:  serviceName,
		maintenance:  false,
		healthStatus: Healthy,
	}, nil
}

// NoneMonitor always returns true (is healthy)
func (m *NoneMonitor) IsHealthy(ctx context.Context) bool { return true }

// HealthStatus implements Monitor interface HealthStatus()
func (m *NoneMonitor) HealthStatus() HealthStatus { return Healthy }

// SetHealthStatus implements Monitor interface SetHealthStatus()
// SetHealthStatus for NoneMonitor doesn't do anything.
func (m *NoneMonitor) SetHealthStatus(health HealthStatus) {}

// ServiceName implements Monitor interface ServiceName()
func (m *NoneMonitor) Service() string { return m.serviceName }

// VipAddress implements Monitor interface VipAddress()
func (m *NoneMonitor) VipAddress() string { return m.vipInfo.address }

// BgpCommunities implements Monitor interface BgpCommunities()
func (m *NoneMonitor) BgpCommunities() []string { return m.vipInfo.bgpCommunities }

// SetMaintenance implements Monitor interface SetMaintenance()
func (m *NoneMonitor) SetMaintenance(isMaintenance bool) { m.maintenance = isMaintenance }

// IsUnderMaintenance implements Monitor interface IsUnderMaintenance()
func (m *NoneMonitor) IsUnderMaintenance() bool { return m.maintenance }

// Type implements Monitor interface Type()
func (m *NoneMonitor) Type() MonitorType { return None }

// String implements Monitor interface String()
func (m *NoneMonitor) String() string {
	str := []string{
		m.Service(),
		m.VipAddress(),
		None.String(),
	}
	return strings.Join(str, ":")
}
