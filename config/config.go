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

package config

import (
	"bytes"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

var AppConfig *Config

// Config represents the *vipcast* configuration.
// Config must contain only pointer values, slices and maps to support
// standardized merging of multiple Config structs into one.
//
// Since this is the format which users use to specify their
// configuration it should be treated as an external API which cannot be
// changed and refactored at will since this will break existing setups.
type Config struct {
	BindAddr        *string        `mapstructure:"bind-addr,omitempty" env:"VIPCAST_API_ADDR" usage:"The address to bind for vipcast API access."`
	StateFile       *string        `mapstructure:"state-file,omitempty" env:"VIPCAST_STATE_FILE" usage:"Path to file to store client apps admin state."`
	MonitorInterval *int           `mapstructure:"monitor-interval,omitempty" env:"VIPCAST_MONITOR_INTERVAL" usage:"Time to wait in seconds between client app healthcheck attempts."`
	CleanupInterval *int           `mapstructure:"cleanup-interval,omitempty" env:"VIPCAST_CLEANUP_INTERVAL" usage:"Time to wait in seconds between flushing out inactive client apps."`
	Consul          *ConsulConfig  `mapstructure:"consul,omitempty"`
	Logging         *LoggingConfig `mapstructure:"log,omitempty"`
}

type ConsulConfig struct {
	HttpAddr         *string   `mapstructure:"addr,omitempty" env:"CONSUL_HTTP_ADDR" usage:"The address of the Consul instance (CONSUL_HTTP_ADDR)."`
	HttpToken        *string   `mapstructure:"token,omitempty" env:"CONSUL_HTTP_TOKEN" usage:"The Consul API token (CONSUL_HTTP_TOKEN)."`
	NodeName         *string   `mapstructure:"node,omitempty" env:"CONSUL_NODE" usage:"Provides access to Node data in Consul (CONSUL_NODE)."`
	AllowStale       *bool     `mapstructure:"allow-stale,omitempty" env:"CONSUL_ALLOW_STALE" usage:"This allows the Consul agent to service the request from its cache (CONSUL_ALLOW_STALE)."`
	ServiceName      *string   `mapstructure:"service-name,omitempty" env:"VIPCAST_CONSUL_SERVICE_NAME" usage:"The vipcast service name as it registered in Consul. This used for vipcast neigbors discovery"`
	ClientSDTags     *[]string `mapstructure:"discovery-tags,omitempty" env:"VIPCAST_CONSUL_CLIENT_SD_TAGS" usage:"Tags to find client apps in Consul."`
	ClientSDInterval *int      `mapstructure:"discovery-interval,omitempty" env:"VIPCAST_CONSUL_CLIENT_SD_INTERVAL" usage:"Time to wait in seconds between query Consul for new client apps."`
}

type LoggingConfig struct {
	Level           *string          `mapstructure:"level,omitempty" env:"VIPCAST_LOG_LEVEL" usage:"Log level"`
	IncludeLocation *bool            `mapstructure:"location,omitempty" env:"VIPCAST_LOG_CALLER" usage:"Log with caller location"`
	Timestamps      *bool            `mapstructure:"timestamps,omitempty" env:"VIPCAST_LOG_DISABLE_TIMESTAMPS" usage:"Log with timestamps"`
	LogLimits       *LogLimitsConfig `mapstructure:"limit,omitempty"`
}

type LogLimitsConfig struct {
	TracePerSecondLimit  *uint32 `mapstructure:"trace,omitempty" env:"VIPCAST_LOG_LIMIT_TRACE" usage:"Limit trace messages to specified rate"`
	DebugPerSecondLimit  *uint32 `mapstructure:"debug,omitempty" env:"VIPCAST_LOG_LIMIT_DEBUG" usage:"Limit debug messages to specified rate"`
	InfoPerSecondLimit   *uint32 `mapstructure:"info,omitempty" env:"VIPCAST_LOG_LIMIT_INFO" usage:"Limit info messages to specified rate"`
	WarnsPerSecondLimit  *uint32 `mapstructure:"warn,omitempty" env:"VIPCAST_LOG_LIMIT_WARN" usage:"Limit warn messages to specified rate"`
	ErrorsPerSecondLimit *uint32 `mapstructure:"error,omitempty" env:"VIPCAST_LOG_LIMIT_ERROR" usage:"Limit error messages to specified rate"`
}

var DefaultConfigYaml = []byte(`
bind-addr: 127.0.0.1:8179
state-file: /tmp/vipcast-state.json
monitor-interval: 5
cleanup-interval: 600
consul:
  addr: http://127.0.0.1:8500
  token: ''
  node: ''
  allow-stale: true
  service-name: vipcast
  discovery-tags: [enable_vipcast]
  discovery-interval: 60
log:
  level: info
  location: true
  timestamps: true
  limit:
    trace: 0
    debug: 0
    info: 0
    warn: 0
    error: 0
`)

func DefaultConfig() *Config {
	var c *Config
	var err error
	v := viper.New()
	v.SetConfigType("yaml")
	if err = v.ReadConfig(bytes.NewBuffer(DefaultConfigYaml)); err != nil {
		panic(err.Error())
	}
	c = &Config{}
	err = v.UnmarshalExact(c,
		func(dc *mapstructure.DecoderConfig) { dc.TagName = "mapstructure" })
	if err != nil {
		panic(err.Error())
	}
	return c
}
