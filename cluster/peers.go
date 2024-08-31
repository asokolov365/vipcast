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

package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/asokolov365/vipcast/lib/consul"
)

// Acceptable peer value hostname:Port, IP:Port, dns+srv://hostname, consul://service-name
func resolvePeers(ctx context.Context, peers []string, myAddr string, waitIfEmpty bool) ([]string, error) {
	var resolvedPeers []string
	res := &net.Resolver{}

	for _, peer := range peers {
		switch {
		case strings.HasPrefix(peer, "dns+srv://"):
			addrs, err := resolveSRV(ctx, peer[len("dns+srv://"):], myAddr, res, waitIfEmpty)
			if err != nil {
				return nil, err
			}
			resolvedPeers = append(resolvedPeers, addrs...)
			continue

		case strings.HasPrefix(peer, "consul://"):
			addrs, err := resolveConsulService(ctx, peer[len("consul://"):], myAddr, waitIfEmpty)
			if err != nil {
				return nil, err
			}
			resolvedPeers = append(resolvedPeers, addrs...)
			continue

		default:
			host, _, err := net.SplitHostPort(peer)
			if err != nil {
				return nil, fmt.Errorf("split host/port for peer %s: %w", peer, err)
			}
			if net.ParseIP(host) == nil {
				addrs, err := resolveHost(ctx, peer, myAddr, res, waitIfEmpty)
				if err != nil {
					return nil, err
				}
				resolvedPeers = append(resolvedPeers, addrs...)

			} else {
				// Assume direct address.
				resolvedPeers = append(resolvedPeers, peer)
			}
		}
	}

	return resolvedPeers, nil
}

// peerAddr must be hostname:port
func resolveHost(ctx context.Context,
	peerAddr string, myAddr string, res *net.Resolver, waitIfEmpty bool) ([]string, error) {
	var result []string

	retryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	peerHost, peerPort, err := net.SplitHostPort(peerAddr)
	if err != nil {
		return nil, fmt.Errorf("split host/port for peer %s: %w", peerAddr, err)
	}

	var lookupErrSpotted bool
	err = retry(2*time.Second, retryCtx.Done(), func() error {
		if lookupErrSpotted {
			// We need to invoke cancel in next run of retry when lookupErrSpotted to preserve LookupIPAddr error.
			cancel()
		}

		addrs, err := res.LookupHost(ctx, peerHost)
		if err != nil {
			lookupErrSpotted = true
			return fmt.Errorf("lookup A record for peer %s: %w", peerHost, err)
		}

		for _, addr := range addrs {
			if addr != myAddr {
				result = append(result, net.JoinHostPort(addr, peerPort))
			}
		}

		if len(result) == 0 {
			if !waitIfEmpty {
				return nil
			}
			return errors.New("empty LookupHost result. Retrying")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveSRV(ctx context.Context,
	dname string, myAddr string, res *net.Resolver, waitIfEmpty bool) ([]string, error) {
	var result []string

	retryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lookupErrSpotted bool
	err := retry(2*time.Second, retryCtx.Done(), func() error {
		if lookupErrSpotted {
			// We need to invoke cancel in next run of retry when lookupErrSpotted to preserve LookupIPAddr error.
			cancel()
		}

		_, addrs, err := res.LookupSRV(ctx, "", "", dname)
		if err != nil {
			lookupErrSpotted = true
			return fmt.Errorf("lookup SRV record for peer %s: %w", dname, err)
		}

		for _, addr := range addrs {
			peerAddr := net.JoinHostPort(
				strings.TrimSuffix(addr.Target, "."),
				strconv.Itoa(int(addr.Port)),
			)
			if peerAddr != myAddr {
				result = append(result, peerAddr)
			}
		}

		if len(result) == 0 {
			if !waitIfEmpty {
				return nil
			}
			return errors.New("empty LookupSRV result. Retrying")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveConsulService(ctx context.Context,
	serviceName string, myAddr string, waitIfEmpty bool) ([]string, error) {
	var result []string

	retryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lookupErrSpotted bool
	err := retry(2*time.Second, retryCtx.Done(), func() error {
		if lookupErrSpotted {
			// We need to invoke cancel in next run of retry when lookupErrSpotted to preserve LookupIPAddr error.
			cancel()
		}

		services, err := consul.ApiClient().CatalogServiceByName(ctx, serviceName)
		if err != nil {
			lookupErrSpotted = true
			return fmt.Errorf("lookup Consul service %s: %w", serviceName, err)
		}

		for _, svc := range services {
			// Address: IP address of the Consul node on which the service is registered.
			// ServiceAddress: IP address of the service host — if empty, node address should be used.
			host := svc.ServiceAddress
			if len(svc.ServiceAddress) == 0 {
				host = svc.Address
			}
			peerAddr := net.JoinHostPort(
				host,
				strconv.Itoa(int(svc.ServicePort)),
			)
			if peerAddr != myAddr {
				result = append(result, peerAddr)
			}
		}

		if len(result) == 0 {
			if !waitIfEmpty {
				return nil
			}
			return errors.New("empty LookupConsul result. Retrying")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// retry executes f() every interval seconds until timeout or no error is returned from f().
func retry(interval time.Duration, stopCh <-chan struct{}, f func() error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var err error
	for {
		if err = f(); err == nil {
			return nil
		}
		select {
		case <-stopCh:
			return err
		case <-ticker.C:
		}
	}
}
