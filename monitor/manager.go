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

	for {
		select {
		case <-ticker.C:
			// Get fresh set of MonitorTargets, they might be updated since last time.
			clients := storage.GetMonitorTargets(mm.consulSDEnabled)
			for _, client := range clients {
				go func(ct context.Context, m Monitor) {
					log := logger.With().
						Str("vip", m.VipAddress()).
						Str("service", m.Service()).
						Logger()

					// Applying jitter to avoid Consul rate limit issues
					jitterMs := fastrand.Uint32n(jitterMaxMs)
					time.Sleep(time.Duration(jitterMs) * time.Millisecond)

					healthCh := make(chan bool, 1)
					healthCh <- m.IsHealthy(ctx)
					select {
					case <-ct.Done():
						<-healthCh // Wait for IsHealthy to return.
						log.Info().Msg("stopping service monitoring during health check")
						return
					case healthy := <-healthCh:
						if healthy {
							m.SetHealthStatus(Healthy)
							log.Debug().Str("monitor", m.Type().String()).Msg("service is healthy")
						} else {
							m.SetHealthStatus(NotHealthy)
							log.Warn().Str("monitor", m.Type().String()).Msg("service is not healthy")
						}
						// default:
						// 	log.Info().Msg("monitoring service")
					}
				}(ctx, client)
			}
		case <-ctx.Done():
			logger.Info().Msg("stopping service monitoring")
			return
		}
	}
}
