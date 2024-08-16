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
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/lib/consul"
	"github.com/asokolov365/vipcast/route"
)

// Metrics
var (
	consulHealthCheckDuration = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="client-healthcheck"}`)
)

// ConsulMonitor implements Monitor interface,
// that checks client service health status via Consul API.
type ConsulMonitor struct {
	*defaultMonitor
}

func NewConsulMonitor(serviceName, vipAddress, bgpCommString string,
	registrar Registrar) (*ConsulMonitor, error) {

	route, err := route.New(vipAddress, bgpCommString)
	if err != nil {
		return nil, err
	}

	return &ConsulMonitor{
		&defaultMonitor{
			lock:         sync.Mutex{},
			serviceName:  serviceName,
			registrar:    registrar,
			route:        route,
			maintenance:  false,
			healthStatus: HealthUndefined,
		},
	}, nil
}

// CheckHealth implements Monitor interface CheckHealth()
func (m *ConsulMonitor) CheckHealth(ctx context.Context) HealthStatus {
	startTime := time.Now()
	defer consulHealthCheckDuration.UpdateDuration(startTime)

	status, err := consul.ApiClient().ServiceHealthStatus(ctx, m.serviceName)
	if err != nil {
		logger.Error().Err(err).
			Str("vip", m.route.Prefix().String()).
			Str("service", m.serviceName).
			Send()
		return HealthUndefined
	}
	if status == "passing" {
		return Healthy
	}
	return NotHealthy
}

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
