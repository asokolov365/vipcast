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
	"fmt"
	"time"

	"github.com/valyala/fastrand"
)

type Manager struct {
	consulSDEnabled bool
	monitorInterval time.Duration
}

func NewManager(withConsul bool, monitorInterval int) *Manager {
	return &Manager{
		consulSDEnabled: withConsul,
		monitorInterval: time.Duration(monitorInterval) * time.Second,
	}
}

const jitterMaxMs uint32 = 1000

func (mm *Manager) DoMonitor(ctx context.Context) {
	ticker := time.NewTicker(mm.monitorInterval)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%v", mm.monitorInterval)).
		Msgf("starting service monitoring")

	timeout := time.Second * 2

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("stopping service monitoring")
			return
		case <-ticker.C:
			// Get fresh set of MonitorTargets, they might be updated since last time.
			clients := storage.GetMonitorTargets(mm.consulSDEnabled)
			if len(clients) == 0 {
				logger.Debug().Msg("no clients for service monitoring")
			}
			for _, client := range clients {
				go func(m Monitor) {
					log := logger.With().
						Str("vip", m.VipAddress()).
						Str("service", m.Service()).
						Logger()

					// Applying jitter to avoid Consul rate limit issues
					jitterMs := fastrand.Uint32n(jitterMaxMs)
					time.Sleep(time.Duration(jitterMs) * time.Millisecond)

					select {
					case <-ctx.Done():
						return
					default:
						timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
						health := m.CheckHealth(timeoutCtx)
						m.SetHealthStatus(health)
						switch health {
						case Healthy:
							log.Debug().Str("monitor", m.Type().String()).Msg("service is healthy")
						case NotHealthy:
							log.Warn().Str("monitor", m.Type().String()).Msg("service is not healthy")
						}
						timeoutCtxCancel()
						return
					}
				}(client)
			}
			// Wait for goroutines to finish and update storage metrics
			time.Sleep(timeout + time.Millisecond)
			storage.UpdateMetrics()
		}
	}
}
