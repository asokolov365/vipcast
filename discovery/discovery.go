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

// Package discovery implements the vipcast Consul service discovery functionality.
package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/lib/consul"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var logger *zerolog.Logger

// Metrics
var (
	consulAgentDuration = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="clients-discovery"}`)
	// consulHealthCheckDuration = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="client-healthcheck"}`)
)

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("discovery")
	}
}

// Discovery provides a service discovery client via Consul API
type Discovery struct {
	consulConfig *config.ConsulConfig
}

// NewDiscovery creates a new Discovery
func NewDiscovery() *Discovery {
	return &Discovery{
		consulConfig: config.AppConfig.Consul,
	}
}

// findClients is used to query for services on a single node.
//
// This finds services that match the provided discovery-tags.
func (d *Discovery) findClients(ctx context.Context) error {
	startTime := time.Now()
	defer consulAgentDuration.UpdateDuration(startTime)

	services, err := consul.ApiClient().NodeServicesByTags(ctx, *d.consulConfig.ClientSDTags)
	if err != nil {
		return err
	}
	discoveredClients := make(map[string]monitor.Monitor, len(services))

	for _, service := range services {
		var (
			vipAddress    string
			bgpCommString string
			monitorString string
		)

		for _, tag := range service.Tags {
			parts := strings.Split(tag, "=")
			if len(parts) != 2 {
				continue
			}
			switch parts[0] {
			case "gocast_vip", "vipcast_vip":
				vipAddress = parts[1]

			case "gocast_vip_communities", "vipcast_bgp_communities":
				bgpCommString = parts[1]

			case "gocast_monitor", "vipcast_monitor":
				monitorString = parts[1]
			}
		}

		// Only VipAddress is mandatory
		if len(vipAddress) > 0 {
			mon, err := monitor.NewMonitor(service.Service, vipAddress, bgpCommString, monitorString, monitor.Discovery)
			if err != nil {
				logger.Warn().Err(err).
					Str("service", mon.Service()).
					Msg("unable to create monitor")
				continue
			}
			discoveredClients[vipAddress] = mon

			logger.Debug().
				Str("service", mon.Service()).
				Str("vip", mon.VipAddress()).
				Str("monitor", mon.Type().String()).
				Msg("found service to vipcast")
		}
	}

	logger.Debug().Msgf("spent %d ms in Consul service discovery", time.Since(startTime).Milliseconds())

	// Update Monitor storage with what is discovered in this pass
	monitor.Storage().Update(discoveredClients)

	return nil
}

// HealthCheck checks if the service is healthy (passing)
// func (c *ConsulApiClient) HealthCheck(ctx context.Context, service string) bool {
// 	startTime := time.Now()
// 	defer consulHealthCheckDuration.UpdateDuration(startTime)

// 	if useLocalAgent {
// 		return c.localHealthCheck(ctx, service)
// 	}
// 	return c.remoteHealthCheck(ctx, service)
// }

const jitterMaxMs uint32 = 1000

func (d *Discovery) DiscoverClients(ctx context.Context) {
	poll := time.Duration(*d.consulConfig.ClientSDInterval) * time.Second
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", *d.consulConfig.ClientSDInterval)).
		Msgf("starting Consul service discovery at %s", *d.consulConfig.HttpAddr)

	timeout := time.Second * 5
	// Start immediately, then loop with ticker
	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
	if err := d.findClients(timeoutCtx); err != nil {
		// Error from Consul, or context timeout:
		logger.Error().Err(err).Send()
	}
	timeoutCtxCancel()

	for {
		select {
		case <-ticker.C:
			go func() {
				jitterMs := fastrand.Uint32n(jitterMaxMs)
				time.Sleep(time.Duration(jitterMs) * time.Millisecond)
				timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
				if err := d.findClients(timeoutCtx); err != nil {
					// Error from Consul, or context timeout:
					logger.Error().Err(err).Send()
				}
				timeoutCtxCancel()
			}()

		case <-ctx.Done():
			logger.Info().Msgf("stopping Consul service discovery at %s", *d.consulConfig.HttpAddr)
			return
		}
	}

}
