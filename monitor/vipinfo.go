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

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

type vipInfo struct {
	address        string
	bgpCommunities []string
}

// ParseVIP parses the specified IP address.
// Acceptable formats are
// IPv4: a.b.c.d, a.b.c.d/32
// IPv6: a.b.c.d.e.f.g.h, a.b.c.d.e.f.g.h/128
func ParseVIP(vip string) (string, error) {
	parts := strings.Split(vip, "/")
	switch len(parts) {
	case 1: // no prefix length
		ipAddr, err := netip.ParseAddr(vip)
		if err != nil {
			return "", fmt.Errorf("%s is not valid VIP: %v", vip, err)
		}
		if ipAddr.Is4() {
			return fmt.Sprintf("%s/32", ipAddr.String()), nil
		} else {
			return fmt.Sprintf("%s/128", ipAddr.String()), nil
		}
	case 2: // with prefix length
		ipPrefix, err := netip.ParsePrefix(vip)
		if err != nil {
			return "", fmt.Errorf("%s is not valid VIP: %v", vip, err)
		}
		if !ipPrefix.IsSingleIP() {
			return "", fmt.Errorf("%s is not valid VIP", vip)
		}
		return vip, nil
	default:
		return "", fmt.Errorf("%s is not valid VIP", vip)
	}
}

// ParseBgpCommunities parses the specified comma separated list of BGP communities.
// A BGP community is a 32-bit value that can be included with a route.
// It can be displayed as a full 32-bit number (0-4,294,967,295) or
// as two 16-bit numbers (0-65535):(0-65535)
func ParseBgpCommunities(communityString string) ([]string, error) {
	communities := strings.Split(communityString, ",")
	for _, c := range communities {
		parts := strings.Split(c, ":")
		switch len(parts) {
		case 1: // classic 32-bit
			_, err := strconv.ParseUint(c, 10, 32)
			if err != nil {
				return []string{}, fmt.Errorf("not valid BGP community: %s: %v", c, err)
			}
		case 2: // new-format (0-65535):(0-65535)
			for _, p := range parts {
				_, err := strconv.ParseUint(p, 10, 16)
				if err != nil {
					return []string{}, fmt.Errorf("not valid BGP community: %s: %v", c, err)
				}
			}
		}
	}
	return communities, nil
}
