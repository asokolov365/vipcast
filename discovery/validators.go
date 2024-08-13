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

package discovery

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/asokolov365/vipcast/monitor"
)

func parseVIP(vip string) (string, error) {
	parts := strings.Split(vip, "/")
	switch len(parts) {
	case 1: // no prefix length
		ipAddr, err := netip.ParseAddr(vip)
		if err != nil {
			logger.Error().Err(err).Str("vip", vip).Msg("unable to parse VIP")
			return "", err
		}
		if ipAddr.Is4() {
			return fmt.Sprintf("%s/32", ipAddr.String()), nil
		} else {
			return fmt.Sprintf("%s/128", ipAddr.String()), nil
		}
	case 2: // with prefix length
		ipPrefix, err := netip.ParsePrefix(vip)
		if err != nil {
			logger.Error().Err(err).Str("vip", vip).Msg("unable to parse VIP")
			return "", err
		}
		if !ipPrefix.IsSingleIP() {
			err = fmt.Errorf("%q is not valid VIP", vip)
			logger.Error().Err(err).Str("vip", vip).Send()
			return "", err
		}
		return vip, nil
	default:
		err := fmt.Errorf("%q is not valid VIP", vip)
		logger.Error().Err(err).Str("vip", vip).Send()
		return "", err
	}
}

func parseBgpCommunities(communityString string) ([]string, error) {
	// A BGP community is a 32-bit value that can be included with a route.
	// It can be displayed as a full 32-bit number (0-4,294,967,295) or
	// as two 16-bit numbers (0-65535):(0-65535)
	communities := strings.Split(communityString, ",")
	for _, c := range communities {
		parts := strings.Split(c, ":")
		switch len(parts) {
		case 1: // classic 32-bit
			_, err := strconv.ParseUint(c, 10, 32)
			if err != nil {
				logger.Error().Err(err).Msgf("not valid BGP community: %q", c)
				return nil, err
			}
		case 2: // new-format (0-65535):(0-65535)
			for _, p := range parts {
				_, err := strconv.ParseUint(p, 10, 16)
				if err != nil {
					logger.Error().Err(err).Msgf("not valid BGP community: %q", p)
					return nil, err
				}
			}
		}
	}
	return communities, nil
}

func parseMonitor(monitorString string) (*monitor.Monitor, error) {
	// valid monitor formats are:
	// "port:tcp:123" , "exec:/local/check.sh", "consul", "off"
	mon := &monitor.Monitor{Type: monitor.NoMonitor}
	switch {
	case strings.HasPrefix(monitorString, "consul"):
		mon.Type = monitor.ConsulMonitor

	case strings.HasPrefix(monitorString, "port"):
		parts := strings.Split(monitorString, ":")
		if len(parts) != 3 {
			err := fmt.Errorf("invalid port monitor, must be port:proto:<port>")
			logger.Error().Err(err).Str("monitor", monitorString).Send()
			return nil, err
		}
		if parts[1] != "tcp" && parts[1] != "udp" {
			err := fmt.Errorf("invalid port monitor, proto must be tcp or udp")
			logger.Error().Err(err).Str("monitor", monitorString).Send()
			return nil, err
		}
		mon.Protocol = parts[1]
		port, err := strconv.ParseUint(parts[2], 10, 16)
		if err != nil {
			err := fmt.Errorf("invalid port monitor, port must be uint16")
			logger.Error().Err(err).Send()
			return nil, err
		}
		mon.Port = int(port)
		mon.Type = monitor.PortMonitor

	case strings.HasPrefix(monitorString, "exec"):
		parts := strings.Split(monitorString, ":")
		if len(parts) != 2 {
			err := fmt.Errorf("invalid exec monitor, must be exec:<command>")
			logger.Error().Err(err).Str("monitor", monitorString).Send()
			return nil, err
		}
		mon.Command = parts[1]
		mon.Type = monitor.ExecMonitor
	case strings.HasPrefix(monitorString, "none"), strings.HasPrefix(monitorString, "off"):
		mon.Type = monitor.NoMonitor
	}
	return mon, nil
}
