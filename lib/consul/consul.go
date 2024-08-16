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

// Package consul wraps Consul API Client functions
package consul

import (
	"context"
	"strings"

	"github.com/hashicorp/consul/api"
)

var consulApiClient *apiClient

// ConsulApiClient wraps the Consul service discovery backend
// and tracks the state of all watched dependencies.
type apiClient struct {
	consul        *api.Client
	agentNodeName string
	localAgent    bool
	queryOpts     *api.QueryOptions
}

// NewApiClient returns a new Consul API client
func NewApiClient(config *api.Config, node string, queryOpts *api.QueryOptions) error {
	consulClient, err := api.NewClient(config)
	if err != nil {
		return err
	}

	localAgent := strings.Contains(config.Address, "localhost") ||
		strings.Contains(config.Address, "127.0.0.1")

	consulApiClient = &apiClient{
		consul:        consulClient,
		agentNodeName: node,
		localAgent:    localAgent,
		queryOpts:     queryOpts,
	}

	return nil
}

func ApiClient() *apiClient {
	if consulApiClient == nil {
		panic("BUG: Consul API client is not initialized")
	}
	return consulApiClient
}

// NodeServicesByTags is used to query services on a single Consul Node,
// filtering the results to include any service that has at least one of the specified tags.
// In other words, a service will be included if its tags contain either tag1, tag2, or tag3, etc.
func (c *apiClient) NodeServicesByTags(ctx context.Context, tags []string) ([]*api.AgentService, error) {
	if err := c.setAgentNodeName(); err != nil {
		return nil, err
	}

	svcList, _, err := c.consul.Catalog().NodeServiceList(
		c.agentNodeName, c.queryOpts.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}

	result := make([]*api.AgentService, 0, len(svcList.Services))

	desiredTags := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		desiredTags[t] = struct{}{}
	}

	for _, service := range svcList.Services {
		for _, tag := range service.Tags {
			if _, ok := desiredTags[tag]; ok {
				result = append(result, service)
				break
			}
		}
	}

	return result, nil
}

// CatalogServiceByName is used to find a service by name.
// It wraps api.Catalog.Service()
func (c *apiClient) CatalogServiceByName(ctx context.Context, service string) ([]*api.CatalogService, error) {
	services, _, err := c.consul.Catalog().Service(
		service, "", c.queryOpts.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	return services, nil
}

// ServiceHealthStatus returns the aggregated health status for all services having the specified name.
// If service is not found, it returns status (critical, nil)
// If the service is found, it returns status (critical|passing|warning), nil)
// In all other cases, it returns status (critical, error)
func (c *apiClient) ServiceHealthStatus(ctx context.Context, service string) (string, error) {
	if c.localAgent {
		return c.ServiceHealthStatusOnAgent(ctx, service)
	}
	return c.ServiceHealthOnCluster(ctx, service)
}

// ServiceHealthStatusOnAgent wraps api.Agent.AgentHealthServiceByNameOpts()
// uses underlying api call: /v1/agent/health/service/name/:service
func (c *apiClient) ServiceHealthStatusOnAgent(ctx context.Context, service string) (string, error) {
	health, _, err := c.consul.Agent().AgentHealthServiceByNameOpts(
		service, c.queryOpts.WithContext(ctx))
	if err != nil {
		return api.HealthCritical, err
	}
	// maintenance > critical > warning > passing
	return health, nil
}

// ServiceHealthOnCluster wraps api.Health.Checks()
// uses underlying api call: /v1/health/checks/:service
func (c *apiClient) ServiceHealthOnCluster(ctx context.Context, service string) (string, error) {
	healthChecks, _, err := c.consul.Health().Checks(
		service, c.queryOpts.WithContext(ctx))
	if err != nil {
		return api.HealthCritical, err
	}
	for _, healthCheck := range healthChecks {
		if healthCheck.Node == c.agentNodeName {
			return healthCheck.Status, nil
		}
	}
	return api.HealthCritical, nil
}

// Trying to get NodeName from a Consul Agent
func (c *apiClient) setAgentNodeName() error {
	if c.agentNodeName != "" {
		return nil
	}
	name, err := c.consul.Agent().NodeName()
	if err != nil {
		return err
	}
	c.agentNodeName = name
	return nil
}
