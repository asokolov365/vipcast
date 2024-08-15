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

// Package monitor implements the vipcast client services monitoring.
package monitor

import (
	"context"
	"strings"

	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/rs/zerolog"
)

var logger *zerolog.Logger

var storage *monStorage

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("monitor")
	}
	storage = NewStorage()
}

// MonitorType defines monitor types.
type MonitorType int8

const (
	None MonitorType = iota
	Consul
	Port
	Exec
	Http
	Dns
)

// String returns a string representation of the monitor type
func (t MonitorType) String() string {
	switch t {
	case Consul:
		return "consul"
	case Port:
		return "port"
	case Exec:
		return "exec"
	case Http:
		return "http"
	case Dns:
		return "dns"
	default:
		return "none"
	}
}

func TypeFromString(t string) MonitorType {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "consul":
		return Consul
	case "port":
		return Port
	case "exec":
		return Exec
	case "http":
		return Http
	case "dns":
		return Dns
	default:
		return None
	}
}

// Monitor is an interface which all monitor backends must implement.
type Monitor interface {
	Service() string
	// IsHealthy runs health check for the service.
	IsHealthy(ctx context.Context) bool
	// HealthStatus returns last known health status of the service.
	HealthStatus() HealthStatus
	// SetHealthStatus sets health status of the service.
	SetHealthStatus(health HealthStatus)
	// VipAddress returns VIP address of the service.
	VipAddress() string
	// BgpCommunities returns list of BGP communities assosiated with the VIP address of the service.
	BgpCommunities() []string
	// SetMaintenance sets the maintenance flag for the service.
	// Services under maintenance will not be monitored and advertised via BGP protocol.
	SetMaintenance(isMaintenance bool)
	// IsUnderMaintenance returns true if the service is under maintenance.
	IsUnderMaintenance() bool
	// String returns string representation of the service.
	String() string
	// Type returns type of the service.
	Type() MonitorType
}

func NewMonitor(serviceName, vipAddress string, bgpCommString string, monitorString string) (Monitor, error) {
	// valid monitorString formats are:
	// "port:tcp:123" , "exec:/local/check.sh", "consul", "http://localhost", "dns:type:dname" "none"
	monitorString = strings.ToLower(strings.TrimSpace(monitorString))
	parts := strings.Split(monitorString, ":")
	monType := TypeFromString(parts[0])

	switch monType {
	case Consul:
		mon, err := NewConsulMonitor(serviceName, vipAddress, bgpCommString)
		if err != nil {
			return nil, err
		}
		return mon, nil
	// case strings.HasPrefix(monitorString, "port"):
	// 	parts := strings.Split(monitorString, ":")
	// 	if len(parts) != 3 {
	// 		err := fmt.Errorf("invalid port monitor, must be port:proto:<port>")
	// 		logger.Error().Err(err).Str("monitor", monitorString).Send()
	// 		return nil, err
	// 	}
	// 	if parts[1] != "tcp" && parts[1] != "udp" {
	// 		err := fmt.Errorf("invalid port monitor, proto must be tcp or udp")
	// 		logger.Error().Err(err).Str("monitor", monitorString).Send()
	// 		return nil, err
	// 	}
	// 	mon.Protocol = parts[1]
	// 	port, err := strconv.ParseUint(parts[2], 10, 16)
	// 	if err != nil {
	// 		err := fmt.Errorf("invalid port monitor, port must be uint16")
	// 		logger.Error().Err(err).Send()
	// 		return nil, err
	// 	}
	// 	mon.Port = int(port)
	// 	mon.Type = monitor.PortMonitor

	// case strings.HasPrefix(monitorString, "exec"):
	// 	parts := strings.Split(monitorString, ":")
	// 	if len(parts) != 2 {
	// 		err := fmt.Errorf("invalid exec monitor, must be exec:<command>")
	// 		logger.Error().Err(err).Str("monitor", monitorString).Send()
	// 		return nil, err
	// 	}
	// 	mon.Command = parts[1]
	// 	mon.Type = monitor.ExecMonitor
	default:
		mon, err := NewNoneMonitor(serviceName, vipAddress, bgpCommString)
		if err != nil {
			return nil, err
		}
		return mon, nil
	}
}

// Storage returns the storage for all known monitors.
func Storage() *monStorage {
	if storage == nil {
		storage = NewStorage()
	}
	return storage
}
