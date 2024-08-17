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

// Package route represents the client VIP and its assosiated BGP communities.
package route

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

type Route struct {
	prefix      *net.IPNet
	communities Communities
}

type Communities []uint32

func New(vipAddress, bgpCommunities string) (*Route, error) {
	prefix, err := ParseVIP(vipAddress)
	if err != nil {
		return nil, err
	}
	comms, err := ParseBgpCommunities(bgpCommunities)
	if err != nil {
		return nil, err
	}
	sort.Slice(comms, func(i, j int) bool { return comms[i] < comms[j] })
	r := &Route{
		prefix:      prefix,
		communities: comms,
	}
	return r, nil
}

// Prefix returns VIP as net.IPNet
func (r *Route) Prefix() *net.IPNet { return r.prefix }

// Communities returns BGP communities associated with VIP
func (r *Route) Communities() Communities { return r.communities }

func (r *Route) Equal(other *Route) bool {
	return r.prefix.IP.Equal(other.prefix.IP) && r.communities.Equal(other.communities)
}

func (c Communities) Equal(other Communities) bool {
	if len(c) != len(other) {
		return false
	}

	for i := range c {
		if c[i] != other[i] {
			return false
		}
	}
	return true
}

// AsStrings returns a string representation of the Communities
// as two 16-bit numbers (0-65535):(0-65535)
func (c Communities) AsStrings() []string {
	if len(c) == 0 {
		return []string{}
	}
	res := make([]string, 0, len(c))
	for _, v := range c {
		p1 := v >> 16
		p2 := uint16(v & (1<<16 - 1))
		s := fmt.Sprintf("%d:%d", p1, p2)
		res = append(res, s)
	}
	return res
}

// ParseVIP parses the specified IP address.
// Acceptable formats are
// IPv4: a.b.c.d, a.b.c.d/32
// IPv6: a.b.c.d.e.f.g.h, a.b.c.d.e.f.g.h/128
func ParseVIP(vip string) (*net.IPNet, error) {
	defaultErr := fmt.Errorf("`%s` is not valid VIP", vip)

	if strings.TrimSpace(vip) == "" {
		return nil, defaultErr
	}

	var cidr string

	parts := strings.Split(vip, "/")

	ip := net.ParseIP(parts[0])
	if ip == nil {
		return nil, defaultErr
	}
	isIPv4 := ip.To4() != nil // if ip is not an IPv4 address, To4 returns nil.

	switch len(parts) {
	case 1: // no prefix length
		if isIPv4 { // IPv4
			cidr = fmt.Sprintf("%s/%d", ip.String(), 32)
		} else { // IPv6
			cidr = fmt.Sprintf("%s/%d", ip.String(), 128)
		}

	case 2: // with prefix length
		pLen, err := strconv.ParseUint(parts[1], 10, 8)
		if err != nil {
			return nil, defaultErr
		}
		if isIPv4 {
			if pLen != 32 {
				return nil, defaultErr
			}
			cidr = fmt.Sprintf("%s/%d", ip.String(), 32)
		} else {
			if pLen != 128 {
				return nil, defaultErr
			}
			cidr = fmt.Sprintf("%s/%d", ip.String(), 128)
		}
	default:
		return nil, defaultErr
	}
	_, net, _ := net.ParseCIDR(cidr)
	return net, nil
}

// ParseBgpCommunities parses the specified comma separated list of BGP communities.
// A BGP community is a 32-bit value that can be included with a route.
// It can be displayed as a full 32-bit number (0-4,294,967,295) or
// as two 16-bit numbers (0-65535):(0-65535)
func ParseBgpCommunities(bgpCommunities string) (Communities, error) {
	comms := strings.Split(bgpCommunities, ",")
	if len(comms[0]) == 0 {
		return Communities{}, nil
	}

	result := make([]uint32, 0, len(comms))
	for _, comm := range comms {
		parts := strings.Split(comm, ":")
		switch len(parts) {
		case 1: // classic 32-bit
			c, err := strconv.ParseUint(comm, 10, 32)
			if err != nil {
				return []uint32{}, fmt.Errorf("not valid BGP community: %s", comm)
			}
			result = append(result, uint32(c))

		case 2: // new-format (0-65535):(0-65535)
			first, err1 := strconv.ParseUint(parts[0], 10, 16)
			second, err2 := strconv.ParseUint(parts[1], 10, 16)
			if err1 != nil || err2 != nil {
				return []uint32{}, fmt.Errorf("not valid BGP community: %s", comm)
			}
			result = append(result, (uint32(first)<<16 | uint32(second)))

		default:
			return []uint32{}, fmt.Errorf("not valid BGP community: %s", comm)
		}
	}
	return result, nil
}
