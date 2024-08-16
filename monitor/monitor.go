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
	"sync"

	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
)

var logger *zerolog.Logger

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("monitor")
	}
	storage = NewStorage()
}

// Monitor is an interface which all monitor backends must implement.
type Monitor interface {
	// Service Name
	Service() string
	// Route returns VIP address and a list of its BGP communities as Route.
	Route() *route.Route
	// VipAddress returns VIP address of the service.
	VipAddress() string
	// Registrar returns what subsys registered Monitor.
	Registrar() Registrar
	// CheckHealth runs health check for the service.
	CheckHealth(ctx context.Context) HealthStatus
	// HealthStatus returns last known health status of the service.
	HealthStatus() HealthStatus
	// SetHealthStatus sets health status of the service.
	SetHealthStatus(health HealthStatus)
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

type defaultMonitor struct {
	lock         sync.Mutex
	serviceName  string
	registrar    Registrar
	route        *route.Route
	maintenance  bool
	healthStatus HealthStatus
}

// ServiceName implements Monitor interface ServiceName()
func (m *defaultMonitor) Service() string { return m.serviceName }

func (m *defaultMonitor) Route() *route.Route { return m.route }

// VipAddress implements Monitor interface VipAddress()
func (m *defaultMonitor) VipAddress() string { return m.route.Prefix().String() }

// Registrar implements Monitor interface Registrar()
func (m *defaultMonitor) Registrar() Registrar { return m.registrar }

// HealthStatus implements Monitor interface HealthStatus()
func (m *defaultMonitor) HealthStatus() HealthStatus { return m.healthStatus }

// SetHealthStatus implements Monitor interface SetHealthStatus()
func (m *defaultMonitor) SetHealthStatus(health HealthStatus) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.healthStatus = health
}

// SetMaintenance implements Monitor interface SetMaintenance()
func (m *defaultMonitor) SetMaintenance(isMaintenance bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.maintenance = isMaintenance
}

// IsUnderMaintenance implements Monitor interface IsUnderMaintenance()
func (m *defaultMonitor) IsUnderMaintenance() bool { return m.maintenance }

func NewMonitor(serviceName, vipAddress, bgpCommString, monitorString string,
	registrar Registrar) (Monitor, error) {
	// valid monitorString formats are:
	// "port:tcp:123" , "exec:/local/check.sh", "consul", "http://localhost", "dns:type:dname" "none"
	monitorString = strings.ToLower(strings.TrimSpace(monitorString))
	parts := strings.Split(monitorString, ":")
	monType := TypeFromString(parts[0])

	switch monType {
	case Consul:
		mon, err := NewConsulMonitor(serviceName, vipAddress, bgpCommString, registrar)
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
		mon, err := NewNoneMonitor(serviceName, vipAddress, bgpCommString, registrar)
		if err != nil {
			return nil, err
		}
		return mon, nil
	}
}
