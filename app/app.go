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
	"strings"

	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/discovery"
	"github.com/asokolov365/vipcast/httpserver"
	"github.com/asokolov365/vipcast/lib/consul"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/hashicorp/consul/api"
	"github.com/rs/zerolog"
)

var (
	logger  *zerolog.Logger
	vipcast *VipCast
)

type VipCast struct {
	apiServer        *httpserver.Server
	serviceDiscovery *discovery.Discovery
	monitorManager   *monitor.Manager
}

// Init initializes the vipcast subsystems (httpserver, monitor, bgp, etc).
func Init() error {
	if logger == nil {
		logger = logging.GetSubLogger("root")
	}
	var (
		serviceDiscovery *discovery.Discovery
	)

	httpserver.Init()
	apiServer := httpserver.NewServer(*config.AppConfig.BindAddr, nil)
	// is Consul SD enabled in config?
	if strings.HasPrefix(*config.AppConfig.Consul.HttpAddr, "http") {
		consulApiConfig := api.DefaultConfig()
		consulApiConfig.Address = *config.AppConfig.Consul.HttpAddr

		if *config.AppConfig.Consul.HttpToken != "" {
			consulApiConfig.Token = *config.AppConfig.Consul.HttpToken
		}
		consulQueryOpts := &api.QueryOptions{
			AllowStale: *config.AppConfig.Consul.AllowStale,
		}

		if err := consul.NewApiClient(consulApiConfig,
			*config.AppConfig.Consul.NodeName, consulQueryOpts); err != nil {
			return err
		}
		discovery.Init()
		serviceDiscovery = discovery.NewDiscovery()
	}

	monitor.Init()
	monitorManager := monitor.NewManager(serviceDiscovery != nil, *config.AppConfig.MonitorInterval)

	vipcast = &VipCast{
		apiServer:        apiServer,
		serviceDiscovery: serviceDiscovery,
		monitorManager:   monitorManager,
	}

	return nil
}

// Run starts the vipcast.
func Run(ctx context.Context) error {
	if vipcast == nil {
		panic("BUG: vipcast is not initialized")
	}

	return vipcast.run(ctx)
}

func (job *VipCast) run(ctx context.Context) error {

	// is Consul enabled?
	if job.serviceDiscovery != nil {
		go job.serviceDiscovery.DiscoverClients(ctx)
	}
	go job.monitorManager.DoMonitor(ctx)
	go monitor.Storage().UpdateMetrics(ctx)
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
