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
	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/lib/consul"
	"github.com/asokolov365/vipcast/route"
)

// Metrics
var (
	consulHealthCheckDuration = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="client-healthcheck"}`)
)

// ConsulMonitor implements Monitor interface,
func NewConsulMonitor(serviceName, vipAddress, bgpCommString string,
	registrar enum.Registrar) (*Monitor, error) {

	route, err := route.New(vipAddress, bgpCommString)
	if err != nil {
		return nil, err
	}

	var healthCheckFunc = func(m *Monitor, ctx context.Context) enum.HealthStatus {
		startTime := time.Now()
		defer consulHealthCheckDuration.UpdateDuration(startTime)

		status, err := consul.ApiClient().ServiceHealthStatus(ctx, m.Service())
		if err != nil {
			logger.Error().Err(err).
				Str("vip", m.Route().Prefix().String()).
				Str("service", m.Service()).
				Send()
			return enum.HealthUndefined
		}
		if status == "passing" {
			return enum.Healthy
		}
		return enum.NotHealthy
	}

	return &Monitor{
		lock:            sync.Mutex{},
		serviceName:     serviceName,
		registrar:       registrar,
		route:           route,
		maintenance:     false,
		healthStatus:    enum.HealthUndefined,
		monitorType:     enum.ConsulMonitor,
		healthCheckFunc: healthCheckFunc,
	}, nil
}
