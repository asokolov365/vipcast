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
)

// NoneMonitor implements Monitor interface,
// that doesn't checks client service health status.
type NoneMonitor struct {
	*defaultMonitor
}

func NewNoneMonitor(serviceName, vipAddress, bgpCommString string,
	registrar Registrar) (*NoneMonitor, error) {
	vip, err := ParseVIP(vipAddress)
	if err != nil {
		return nil, err
	}

	bgpCommunities, err := ParseBgpCommunities(bgpCommString)
	if err != nil {
		return nil, err
	}

	return &NoneMonitor{
		&defaultMonitor{
			lock:         sync.Mutex{},
			serviceName:  serviceName,
			registrar:    registrar,
			vipInfo:      &vipInfo{address: vip, bgpCommunities: bgpCommunities},
			maintenance:  false,
			healthStatus: Healthy,
		},
	}, nil
}

// NoneMonitor always returns true (is healthy)
func (m *NoneMonitor) IsHealthy(ctx context.Context) bool { return true }

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
