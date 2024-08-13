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
	"fmt"

	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/rs/zerolog"
)

var logger *zerolog.Logger

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("monitor")
	}
}

// MonitorType defines monitor types.
type MonitorType int8

const (
	// ConsulMonitor defines consul monitor type.
	ConsulMonitor MonitorType = iota
	// PortMonitor defines port monitor type.
	PortMonitor
	// ExecMonitor defines exec monitor type.
	ExecMonitor
	// NoMonitor defines no monitor type.
	NoMonitor
)

func (t MonitorType) String() string {
	switch t {
	case ConsulMonitor:
		return "consul"
	case PortMonitor:
		return "port"
	case ExecMonitor:
		return "exec"
	default:
		return "none"
	}
}

type Monitor struct {
	Type     MonitorType
	Protocol string
	Port     int
	Command  string
}

func (m1 *Monitor) Equal(m2 *Monitor) bool {
	switch m1.Type {
	case PortMonitor:
		return fmt.Sprintf("%s:%s:%d", m1.Type.String(), m1.Protocol, m1.Port) == fmt.Sprintf("%s:%s:%d", m2.Type.String(), m2.Protocol, m2.Port)
	case ExecMonitor:
		return fmt.Sprintf("%s:%s", m1.Type.String(), m1.Command) == fmt.Sprintf("%s:%s", m2.Type.String(), m2.Command)
	default:
		return m1.Type.String() == m2.Type.String()
	}
}

func (m *Monitor) String() string {
	switch m.Type {
	case PortMonitor:
		return fmt.Sprintf("%s:%s:%d", m.Type.String(), m.Protocol, m.Port)
	case ExecMonitor:
		return fmt.Sprintf("%s:%s", m.Type.String(), m.Command)
	default:
		return m.Type.String()
	}
}
