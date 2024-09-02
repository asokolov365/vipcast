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

	"github.com/VictoriaMetrics/metrics"
	"github.com/valyala/fastrand"
	"golang.org/x/sync/errgroup"
)

var mgr *monitorManager

// Metrics
var (
	monitoringCycleDuration = metrics.NewHistogram(`vipcast_monitoring_cycle_duration_seconds`)
)

type monitorManager struct {
	consulSDEnabled bool
	monitorInterval time.Duration
}

// Registry returns the local Monitor Registry.
func Manager() *monitorManager {
	if mgr == nil {
		panic("BUG: Monitor Manager is not initialized")
	}
	return mgr
}

func newManager(consulSDEnabled bool, monitorInterval int) *monitorManager {
	return &monitorManager{
		consulSDEnabled: consulSDEnabled,
		monitorInterval: time.Duration(monitorInterval) * time.Second,
	}
}

const jitterMaxMs uint32 = 1000

func (mm *monitorManager) DoMonitor(ctx context.Context) {
	ticker := time.NewTicker(mm.monitorInterval)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%v", mm.monitorInterval)).
		Msgf("starting service monitoring")

	timeout := time.Second * 2

	for {
		select {
		case <-ticker.C:
			startTime := time.Now()
			logger.Debug().Msgf("running service monitoring cycle")

			// Get fresh set of Monitors, they might be updated since last time.
			clients := Registry().GetActiveMonitors(mm.consulSDEnabled)
			if len(clients) == 0 {
				logger.Debug().Msg("no clients for service monitoring")
			}

			g, ctx := errgroup.WithContext(ctx)

			for _, client := range clients {
				m := client
				g.Go(func() error {
					log := logger.With().
						Str("vip", m.VipAddress()).
						Str("service", m.Service()).
						Str("monitor", m.Type().String()).
						Logger()

					// Applying jitter to avoid Consul rate limit issues
					jitterMs := fastrand.Uint32n(jitterMaxMs)
					time.Sleep(time.Duration(jitterMs) * time.Millisecond)

					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
						defer timeoutCtxCancel()
						health, err := m.healthCheckFunc(m, timeoutCtx)
						m.SetHealthStatus(health)
						if err != nil {
							log.Err(err).Send()
						}
						log.Debug().Msg(health.String())
						return nil
					}
				})
			}
			// Wait for goroutines to finish and update metrics
			err := g.Wait()
			if err != nil {
				logger.Err(err).Msg("monitoring goroutine exits with error")
			}
			monitoringCycleDuration.UpdateDuration(startTime)
			Registry().UpdateMetrics()

		case <-ctx.Done():
			logger.Info().Msg("stopping service monitoring")
			return
		}
	}
}
