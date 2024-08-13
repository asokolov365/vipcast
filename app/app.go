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

// Package app implements a vipcast application.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/discovery"
	"github.com/asokolov365/vipcast/httpserver"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var (
	logger  *zerolog.Logger
	vipcast *VipCast
)

// Init initializes the vipcast jobs (ApiHttpServer, Monitors, BGP client, etc).
func Init() error {
	if logger == nil {
		logger = logging.GetSubLogger("root")
	}

	var consulApi *discovery.ConsulApiClient
	var err error

	httpserver.Init()
	monitor.Init()
	apiServer := httpserver.NewServer(*config.AppConfig.BindAddr, nil)

	// Consul enabled
	if strings.HasPrefix(*config.AppConfig.Consul.HttpAddr, "http") {
		discovery.Init()
		consulApi, err = discovery.NewConsulApiClient(config.AppConfig.Consul)
		if err != nil {
			return err
		}
	}

	vipcast = &VipCast{
		apiServer: apiServer,
		consulApi: consulApi,
	}

	return nil
}

type VipCast struct {
	apiServer *httpserver.Server
	consulApi *discovery.ConsulApiClient
}

// Run starts the vipcast.
func Run(ctx context.Context) error {
	if vipcast == nil {
		panic("BUG: vipcast is not initialized")
	}

	return vipcast.run(ctx)
}

func (job *VipCast) run(ctx context.Context) error {

	// if Consul enabled
	if job.consulApi != nil {
		go job.consulApi.Discover(ctx)
		go job.monitorConsulClients(ctx)
	}

	job.apiServer.Serve(ctx)

	// neighbors, err := job.consulApi.NeighborServiceList(ctx)
	// if err != nil {
	// 	return err
	// }
	// for _, s := range neighbors {
	// 	fmt.Printf("%s: %s:%d\n", s.Node, s.ServiceAddress, s.ServicePort)
	// 	fmt.Printf("healthy: %t\n", job.consulApi.HealthCheck(s.ServiceName))
	// }

	select {
	case <-ctx.Done():
		logger.Info().Msg("stopping vipcast")
	default:
	}

	return nil
}

const jitterMaxMs uint32 = 1000

func (job *VipCast) monitorConsulClients(ctx context.Context) {
	poll := time.Duration(*config.AppConfig.MonitorInterval) * time.Second
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", *config.AppConfig.MonitorInterval)).
		Msgf("starting monitoring Consul services")

	for {
		select {
		case <-ticker.C:
			// Get fresh set of Clients, they might be updated since last time.
			clients := monitor.GetConsulMonitorTargets()
			for _, client := range clients {
				jitterMs := fastrand.Uint32n(jitterMaxMs)
				time.Sleep(time.Duration(jitterMs) * time.Millisecond)

				go func(ct context.Context, cl *monitor.ClientVIP) {
					log := logger.With().Str("vip", cl.VipAddress).Str("service", cl.ServiceName).Logger()
					healthCh := make(chan bool, 1)
					healthCh <- job.consulApi.HealthCheck(ct, cl.ServiceName)
					select {
					case <-ct.Done():
						<-healthCh // Wait for HealthCheck to return.
						log.Info().Msg("stopping Consul monitoring")
						return
					case healthy := <-healthCh:
						if healthy {
							log.Debug().Msg("service is healthy")
						} else {
							log.Warn().Msg("service is not healthy")
						}
						// default:
						// 	log.Info().Msg("monitoring Consul service")
					}
				}(ctx, client)
			}
		case <-ctx.Done():
			logger.Info().Msg("stopping monitoring Consul services")
			return
		}
	}
}
