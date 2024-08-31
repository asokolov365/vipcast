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
	"time"

	"github.com/asokolov365/vipcast/cluster"
	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/discovery"
	"github.com/asokolov365/vipcast/httpserver"
	"github.com/asokolov365/vipcast/lib/consul"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/maintenance"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/asokolov365/vipcast/registry"
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
func Init(ctx context.Context) error {
	if logger == nil {
		logger = logging.GetSubLogger("root")
	}

	if err := cluster.Init(ctx, config.AppConfig.Cluster); err != nil {
		logger.Fatal().Err(err)
		return err
	}

	httpserver.Init()
	apiServer := httpserver.NewServer(*config.AppConfig.BindAddr)

	var serviceDiscovery *discovery.Discovery

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

func (vc *VipCast) run(ctx context.Context) error {

	cluster.Run()

	// is Consul enabled?
	if vc.serviceDiscovery != nil {
		go vc.serviceDiscovery.DiscoverClients(ctx)
		// go job.serviceDiscovery.DiscoverNeighbors(ctx)
	}
	go vc.monitorManager.DoMonitor(ctx)
	go registry.Registry().BroadcastRegistry(ctx)
	go maintenance.MaintDB().BroadcastMaintenance(ctx)
	go maintenance.MaintDB().Cleanup(ctx, *config.AppConfig.CleanupInterval)
	vc.apiServer.Serve(ctx)

	if err := cluster.GracefulShutdown(5 * time.Second); err != nil {
		logger.Err(err).Send()
	}

	select {
	case <-ctx.Done():
		logger.Info().Msg("stopping vipcast")
	default:
	}

	return nil
}
