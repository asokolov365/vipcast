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

	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
)

var logger *zerolog.Logger

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("monitor")
	}
}

// Monitor is an interface which all monitor backends must implement.
type Monitor struct {
	lock            sync.Mutex
	serviceName     string
	registrar       enum.Registrar
	route           *route.Route
	maintenance     bool
	healthCheckFunc HealthCheckFunc
	healthStatus    enum.HealthStatus
	monitorType     enum.MonitorType
	monitorParams   string
}

type HealthCheckFunc func(m *Monitor, ctx context.Context) enum.HealthStatus

// Service returns name of the service.
func (m *Monitor) Service() string { return m.serviceName }

// Route returns *route.Route object (VIP and VIP's associated BGP communities)
func (m *Monitor) Route() *route.Route { return m.route }

// VipAddress returns string representation of VIP address of the service.
func (m *Monitor) VipAddress() string { return m.route.Prefix().String() }

// Registrar returns what subsys has registered Monitor.
func (m *Monitor) Registrar() enum.Registrar { return m.registrar }

// HealthStatus returns last known health status of the service.
func (m *Monitor) HealthStatus() enum.HealthStatus { return m.healthStatus }

// SetHealthStatus sets health status of the service.
func (m *Monitor) SetHealthStatus(health enum.HealthStatus) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.healthStatus = health
}

// SetMaintenance sets the maintenance flag for the service.
// Services under maintenance will not be monitored and advertised via BGP protocol.
func (m *Monitor) SetMaintenance(mode enum.MaintenanceMode) {
	m.lock.Lock()
	defer m.lock.Unlock()

	logger.Info().
		Str("vip", m.VipAddress()).
		Str("service", m.Service()).
		Msgf("maintenance mode %s", mode.String())
	m.maintenance = (mode == enum.MaintenanceOn)
}

// IsUnderMaintenance returns true if the service is under maintenance.
func (m *Monitor) IsUnderMaintenance() bool { return m.maintenance }

// String returns string representation of the service.
func (m *Monitor) String() string {
	str := []string{
		m.Service(),
		m.VipAddress(),
		m.monitorType.String(),
	}
	return strings.Join(str, ":")
}

// Params returns string representation of monitoring parameters.
func (m *Monitor) Params() string { return m.monitorParams }

// Type returns type of Monitor for the service.
func (m *Monitor) Type() enum.MonitorType { return m.monitorType }

// Equal returns if Monitor is equal to other Monitor.
func (m *Monitor) Equal(other *Monitor) bool {
	if m.monitorType != other.Type() {
		return false
	}
	if !m.route.Equal(other.Route()) {
		return false
	}
	if m.monitorParams != other.Params() {
		return false
	}
	return true
}

func NewMonitor(serviceName, vipAddress, bgpCommString, monitorString string,
	registrar enum.Registrar) (*Monitor, error) {
	// valid monitorString formats are:
	// "port:tcp:123" , "exec:/local/check.sh", "consul", "http://localhost", "dns:type:dname" "none"
	monitorString = strings.ToLower(strings.TrimSpace(monitorString))
	parts := strings.Split(monitorString, ":")
	monType := enum.MonitorTypeFromString(parts[0])

	switch monType {
	case enum.ConsulMonitor:
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
