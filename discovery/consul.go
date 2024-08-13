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
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/hashicorp/consul/api"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
)

var (
	logger            *zerolog.Logger
	agentNodeName     string
	useLocalAgent     bool
	vipcastEnableTags map[string]struct{}
	defaultQueryOpts  *api.QueryOptions
)

// Metrics
var (
	consulAgentDuration       = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="clients-discovery"}`)
	consulHealthCheckDuration = metrics.NewHistogram(`vipcast_consul_interaction_duration_seconds{action="client-healthcheck"}`)
)

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("discovery")
	}
	// This is for fast tag search
	vipcastEnableTags = make(map[string]struct{}, len(*config.AppConfig.Consul.ClientSDTags))
	for _, tag := range *config.AppConfig.Consul.ClientSDTags {
		vipcastEnableTags[tag] = struct{}{}
	}
	// Agent Node Name from config
	agentNodeName = *config.AppConfig.Consul.NodeName
	// If the address contains "localhost", then we presume that
	// the local agent is to be used for healthcheck.
	useLocalAgent = strings.Contains(*config.AppConfig.Consul.HttpAddr, "localhost") ||
		strings.Contains(*config.AppConfig.Consul.HttpAddr, "127.0.0.1")
	// Populate Consul API QueryOptions with AllowStale
	// We'll add context later when we run queries
	defaultQueryOpts = &api.QueryOptions{
		AllowStale: *config.AppConfig.Consul.AllowStale,
	}
}

// ConsulApiClient wraps the Consul service discovery backend
// and tracks the state of all watched dependencies.
type ConsulApiClient struct {
	client *api.Client
	config *config.ConsulConfig
}

func NewConsulApiClient(config *config.ConsulConfig) (*ConsulApiClient, error) {
	consulApiConfig := api.DefaultConfig()
	consulApiConfig.Address = *config.HttpAddr

	if *config.HttpToken != "" {
		consulApiConfig.Token = *config.HttpToken
	}

	consulApiClient, err := api.NewClient(consulApiConfig)
	if err != nil {
		return nil, err
	}

	return &ConsulApiClient{
		client: consulApiClient,
		config: config,
	}, nil
}

// findClients is used to query for services on a single node.
//
// This finds services that match the provided discovery-tags.
func (c *ConsulApiClient) findClients(ctx context.Context) error {
	if err := c.setConsulAgentNodeName(); err != nil {
		logger.Fatal().Err(err).Msgf("unable to get Consul Agent.NodeName from %q", *c.config.HttpAddr)
		return err
	}

	startTime := time.Now()
	defer consulAgentDuration.UpdateDuration(startTime)

	list, _, err := c.client.Catalog().NodeServiceList(
		agentNodeName, defaultQueryOpts.WithContext(ctx),
	)
	if err != nil {
		// logger.Error().Err(err).Msgf("unable to get Consul Catalog from %q", *c.config.HttpAddr)
		return err
	}
	foundClients := make(map[string]*monitor.ClientVIP, len(list.Services))

SERVICE_LOOP:
	for _, service := range list.Services {
		serviceMatch := false

		vipInfo := &monitor.ClientVIP{
			VipAddress:     "",
			BgpCommunities: []string{},
			Monitor:        &monitor.Monitor{Type: monitor.NoMonitor},
			ServiceName:    "undefined",
			ServiceAddress: "",
			ServicePort:    0,
			ServiceTags:    []string{},
		}
		tags := make([]string, 0, len(service.Tags))

		for _, tag := range service.Tags {
			if _, ok := vipcastEnableTags[tag]; ok {
				serviceMatch = true
				tags = append(tags, tag)
				continue
			}
			parts := strings.Split(tag, "=")
			if len(parts) != 2 {
				continue
			}
			switch parts[0] {
			case "gocast_vip", "vipcast_vip":
				vip, err := parseVIP(parts[1])
				if err != nil {
					continue SERVICE_LOOP
				}
				vipInfo.VipAddress = vip
				tags = append(tags, tag)

			case "gocast_vip_communities", "vipcast_bgp_communities":
				bgpCommunities, err := parseBgpCommunities(parts[1])
				if err != nil {
					continue SERVICE_LOOP
				}
				vipInfo.BgpCommunities = bgpCommunities
				tags = append(tags, tag)

			case "gocast_monitor", "vipcast_monitor":
				m, err := parseMonitor(parts[1])
				if err != nil {
					continue SERVICE_LOOP
				}
				vipInfo.Monitor = m
				tags = append(tags, tag)
			}
		}
		// if service matched, only VipAddress is mandatory
		if serviceMatch && len(vipInfo.VipAddress) > 0 {
			// Adding Consul service Info
			vipInfo.ServiceName = service.Service
			vipInfo.ServiceAddress = service.Address
			vipInfo.ServicePort = service.Port
			vipInfo.ServiceTags = tags
			foundClients[vipInfo.VipAddress] = vipInfo

			logger.Debug().
				Str("service", vipInfo.ServiceName).
				Str("vip", vipInfo.VipAddress).
				Strs("tags", vipInfo.ServiceTags).
				Msg("found service to vipcast")
		}
	}

	logger.Debug().Msgf("spent %d ms in Consul service discovery", time.Since(startTime).Milliseconds())

	// Update ClientVIPs with what is discovered in this pass
	monitor.Update(foundClients)

	return nil
}

// NeighborServiceList is used to discovery vipcast neighbors,
// It finds services that match the provided service-name.
// vipcast cluster is the set of vipcast services that share the same service name in Consul.
// This is used to turn on/off maintenance mode for a VIP on all vipcast instances in the cluster
func (c *ConsulApiClient) NeighborServiceList(ctx context.Context) ([]*api.CatalogService, error) {
	neighbors, _, err := c.client.Catalog().Service(
		*c.config.ServiceName, "", defaultQueryOpts.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	return neighbors, nil
}

// HealthCheck checks if the service is healthy (passing)
func (c *ConsulApiClient) HealthCheck(ctx context.Context, service string) bool {
	startTime := time.Now()
	defer consulHealthCheckDuration.UpdateDuration(startTime)

	if useLocalAgent {
		return c.localHealthCheck(ctx, service)
	}
	return c.remoteHealthCheck(ctx, service)
}

// localHealthCheck uses underlying api call: /v1/agent/health/service/name/:service
func (c *ConsulApiClient) localHealthCheck(ctx context.Context, service string) bool {
	health, _, err := c.client.Agent().AgentHealthServiceByNameOpts(
		service, defaultQueryOpts.WithContext(ctx))
	if err != nil {
		logger.Error().Err(err).Str("service", service).Msg("unable to retrieve service health status from local Consul agent")
		return false
	}
	// maintenance > critical > warning > passing
	return health == "passing"
}

// remoteHealthCheck uses underlying api call: /v1/health/checks/:service
func (c *ConsulApiClient) remoteHealthCheck(ctx context.Context, service string) bool {
	health, _, err := c.client.Health().Checks(
		service, defaultQueryOpts.WithContext(ctx))
	if err != nil {
		logger.Error().Err(err).Str("service", service).Msg("unable to retrieve service health status from Consul cluster")
		return false
	}
	// maintenance > critical > warning > passing
	return health.AggregatedStatus() == "passing"
}

func (c *ConsulApiClient) setConsulAgentNodeName() error {
	if agentNodeName != "" {
		return nil
	}
	// Attempt to get NodeName from Consul Agent
	name, err := c.client.Agent().NodeName()
	if err != nil {
		return err
	}
	agentNodeName = name
	return nil
}

const jitterMaxMs uint32 = 1000

func (c *ConsulApiClient) Discover(ctx context.Context) {
	poll := time.Duration(*c.config.ClientSDInterval) * time.Second
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	logger.Info().Str("interval", fmt.Sprintf("%ds", *c.config.ClientSDInterval)).
		Msgf("starting discovery at %s", *c.config.HttpAddr)

	errCh := make(chan error, 1)
	go func() { errCh <- c.findClients(ctx) }()
	select {
	case <-ctx.Done():
		<-errCh // Wait for findClients to return.
		logger.Info().Msgf("stopping Consul service discovery at %s", *c.config.HttpAddr)
		return
	case err := <-errCh:
		logger.Error().Err(err).Msg("findClients error")
	default:
	}

	for {
		select {
		case <-ticker.C:
			errCh := make(chan error, 1)
			go func() {
				jitterMs := fastrand.Uint32n(jitterMaxMs)
				time.Sleep(time.Duration(jitterMs) * time.Millisecond)

				errCh <- c.findClients(ctx)
			}()
			select {
			case <-ctx.Done():
				<-errCh // Wait for findClients to return.
				logger.Info().Msgf("stopping Consul service discovery at %s", *c.config.HttpAddr)
				return
			case err := <-errCh:
				logger.Error().Err(err).Msg("findClients error")
			default:
			}

		case <-ctx.Done():
			logger.Info().Msgf("stopping Consul service discovery at %s", *c.config.HttpAddr)
			return
		}
	}

}
