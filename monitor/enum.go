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

import "strings"

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
		return "not healthy"
	case Healthy:
		return "healthy"
	default:
		return "undefined"
	}
}

// Registrar defines the reporters, what subsys registered Monitor.
// It is neccessary to distinguish Monitors registered via http API vs Discovered via Consul.
type Registrar int8

const (
	Admin Registrar = iota
	Discovery
)
