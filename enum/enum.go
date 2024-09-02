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

package enum

import "strings"

const (
	RegistryApiPath    = "/api/v1/registry"
	MaintenanceApiPath = "/api/v1/maintenance"
)

// MonitorType defines monitor types.
type MonitorType int8

const (
	ErrorMonitor MonitorType = iota
	ConsulMonitor
	PortMonitor
	ExecMonitor
	HttpMonitor
	DnsMonitor
	NoneMonitor
)

// String returns a string representation of the monitor type
func (t MonitorType) String() string {
	switch t {
	case ConsulMonitor:
		return "consul"
	case PortMonitor:
		return "port"
	case ExecMonitor:
		return "exec"
	case HttpMonitor:
		return "http"
	case DnsMonitor:
		return "dns"
	case NoneMonitor:
		return "none"
	default:
		return "error"
	}
}

func MonitorTypeFromString(t string) MonitorType {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "consul":
		return ConsulMonitor
	case "port":
		return PortMonitor
	case "exec":
		return ExecMonitor
	case "http":
		return HttpMonitor
	case "dns":
		return DnsMonitor
	case "none":
		return NoneMonitor
	default:
		return ErrorMonitor
	}
}

// Health defines possible service health statuses.
type HealthStatus int8

const (
	NotHealthy HealthStatus = iota
	Healthy
	HealthUndefined
)

// String returns a string representation of the Health status
func (t HealthStatus) String() string {
	switch t {
	case NotHealthy:
		return "service is not healthy"
	case Healthy:
		return "service is healthy"
	default:
		return "service health is undefined"
	}
}

// Registrar defines the reporters, what subsys registered Monitor.
// It is neccessary to distinguish Monitors registered via http API vs Discovered via Consul.
type Registrar int8

const (
	AdminRegistrar Registrar = iota
	DiscoveryRegistrar
)

type MaintenanceMode int8

const (
	MaintenanceOff MaintenanceMode = iota
	MaintenanceOn
	MaintenanceUndefined
)

// String returns a string representation of the Health status
func (m MaintenanceMode) String() string {
	switch m {
	case MaintenanceOff:
		return "off"
	case MaintenanceOn:
		return "on"
	default:
		return "undefined"
	}
}
