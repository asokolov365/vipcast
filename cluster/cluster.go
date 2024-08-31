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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/hashicorp/serf/serf"
	"github.com/rs/zerolog"
	"github.com/valyala/fastrand"
	"golang.org/x/sync/errgroup"
)

var logger *zerolog.Logger

var cl *vipcastCluster

func Init(ctx context.Context, config *config.ClusterConfig) error {
	var err error
	if logger == nil {
		logger = logging.GetSubLogger("cluster")
	}
	if cl == nil {
		cl, err = initCluster(ctx, config)
	}
	return err
}

type vipcastCluster struct {
	bindAddr       string
	bindPort       int
	advertiseAddr  string
	advertisePort  int
	bootstrapPeers []string
	peersToNotify  int
	serfConfig     *serf.Config
	serfInstance   *serf.Serf
}

// VipcastCluster returns initialized instance of the vipcast cluster.
func VipcastCluster() *vipcastCluster {
	if cl == nil || cl.serfInstance == nil {
		panic("BUG: vipcast cluster is not initialized")
	}
	return cl
}

func initCluster(ctx context.Context, config *config.ClusterConfig) (*vipcastCluster, error) {
	bindAddr, bindPortStr, err := net.SplitHostPort(*config.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address: %w", err)
	}
	bindPort, err := strconv.Atoi(bindPortStr)
	if err != nil {
		return nil, fmt.Errorf("address %s: invalid port: %w", *config.BindAddr, err)
	}

	var advertiseAddr string
	var advertisePort int
	if *config.AdvertiseAddr != "" {
		var advertisePortStr string
		advertiseAddr, advertisePortStr, err = net.SplitHostPort(*config.AdvertiseAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid advertise address: %w", err)
		}
		advertisePort, err = strconv.Atoi(advertisePortStr)
		if err != nil {
			return nil, fmt.Errorf("address %s: invalid port: %w", *config.AdvertiseAddr, err)
		}
	}

	resolvedPeers, err := resolvePeers(ctx, *config.Peers, *config.AdvertiseAddr, *config.WaitForPeers)
	if err != nil {
		return nil, fmt.Errorf("resolve peers: %w", err)
	}
	logger.Debug().Strs("peers", resolvedPeers).Msg("resolved peers to following addresses")

	// Initial validation of user-specified advertise address.
	addr, err := findAdvertiseAddress(bindAddr, advertiseAddr, *config.InsecureAdvertise)
	if err != nil {
		logger.Warn().Err(err).Msg("couldn't deduce an advertise address")

	} else if hasNonlocalAddr(resolvedPeers) && isUnroutableAddr(addr.String()) {
		logger.Warn().Str("addr", addr.String()).Msg("this node advertises itself on an unroutable address")
		logger.Warn().Msg("this node will be unreachable in the cluster")
		logger.Warn().Msg("provide --cluster.advertise-addr as a routable IP address or hostname")

	} else if isUnspecifiedAddr(*config.BindAddr) && advertiseAddr == "" {
		// memberlist doesn't advertise properly when the bind address is empty or unspecified.
		logger.Info().Str("addr", addr.String()).Int("port", bindPort).Msg("setting advertise address explicitly")
		advertiseAddr = addr.String()
		advertisePort = bindPort
	}

	// serfLogger := stdlog.Default()
	// serfLogger.SetFlags(0)
	// serfLogger.SetOutput(logging.GetSubLogger("serf"))

	serfConfig := serf.DefaultConfig()
	serfConfig.Init()
	// serfConfig.Logger = serfLogger
	serfConfig.LogOutput = io.Discard
	serfConfig.MemberlistConfig.LogOutput = io.Discard
	serfConfig.MemberlistConfig.BindAddr = bindAddr
	serfConfig.MemberlistConfig.BindPort = bindPort
	serfConfig.MemberlistConfig.AdvertiseAddr = advertiseAddr
	serfConfig.MemberlistConfig.AdvertisePort = advertisePort
	serfConfig.MemberlistConfig.EnableCompression = true

	logger.Info().
		Str("instance", net.JoinHostPort(advertiseAddr, strconv.Itoa(advertisePort))).
		Msg("creating vipcast cluster instance")
	serfInstance, err := serf.Create(serfConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create vipcast cluster instance: %w", err)
	}

	return &vipcastCluster{
		bindAddr:       bindAddr,
		bindPort:       bindPort,
		advertiseAddr:  advertiseAddr,
		advertisePort:  advertisePort,
		bootstrapPeers: resolvedPeers,
		peersToNotify:  *config.PeersToNotify,
		serfConfig:     serfConfig,
		serfInstance:   serfInstance,
	}, nil
}

func (c *vipcastCluster) LocalMember() serf.Member {
	return c.serfInstance.LocalMember()
}

func (c *vipcastCluster) OtherMembers() []serf.Member {
	allMembers := c.serfInstance.Members()
	otherMembers := make([]serf.Member, 0, len(allMembers))
	for _, m := range allMembers {
		if m.Name == c.serfInstance.LocalMember().Name || m.Status != serf.StatusAlive {
			continue
		}
		otherMembers = append(otherMembers, m)
	}
	return otherMembers
}

func (c *vipcastCluster) NotifyCluster(ctx context.Context, method, path string, body []byte) {
	otherMembers := c.OtherMembers()
	if len(otherMembers) == 0 {
		logger.Warn().Msg("vipcast cluster has no other alive members")
		return
	}

	g, ctx := errgroup.WithContext(ctx)
	if len(otherMembers) <= c.peersToNotify {
		for _, member := range otherMembers {
			url := fmt.Sprintf("http://%s:%d%s", member.Addr.String(), member.Port, path)
			g.Go(func() error {
				return notifyClusterMember(ctx, method, url, body)
			})
		}
	} else {
		randIdx := int(fastrand.Uint32n(uint32(len(otherMembers))))
		for i := 0; i < c.peersToNotify; i++ {
			member := otherMembers[(randIdx+i)%len(otherMembers)]
			url := fmt.Sprintf("http://%s:%d%s", member.Addr.String(), member.Port, path)
			g.Go(func() error {
				return notifyClusterMember(ctx, method, url, body)
			})
		}
	}

	err := g.Wait()
	if err != nil {
		logger.Err(err).Msg("error notifying other members")
	}
}

func Run() {
	if cl == nil || cl.serfInstance == nil {
		panic("BUG: vipcast cluster is not initialized")
	}

	logger.Info().Msg("joining vipcast cluster")

	_, err := cl.serfInstance.Join(cl.bootstrapPeers, false)
	if err != nil || len(cl.bootstrapPeers) == 0 {
		logger.Warn().Err(err).Msg("unable to join vipcast cluster, starting own")
	}
}

func GracefulShutdown(timeout time.Duration) error {
	if cl == nil || cl.serfInstance == nil {
		panic("BUG: vipcast cluster is not initialized")
	}

	retryTicker := time.NewTicker(1 * time.Second)
	defer retryTicker.Stop()

	deadLine := time.Now().Add(timeout)
	deadLineCtx, deadLineCtxCancel := context.WithDeadline(context.Background(), deadLine)
	defer deadLineCtxCancel()

	attempt := 1

	// Trying to leave immediately, then retry by ticker
	logger.Info().Int("attempt", attempt).Msg("gracefully leaving vipcast cluster")
	err := cl.serfInstance.Leave()
	if err == nil {
		return nil
	}
	logger.Err(err).Msg("serf.Leave() failed")

	for {
		select {
		case <-deadLineCtx.Done():
			logger.Warn().Msg("forcefully stopping vipcast cluster instance")
			if err := cl.serfInstance.Shutdown(); err != nil {
				logger.Err(err).Msg("serf.Shutdown() failed")
				return err
			}
			return nil

		case <-retryTicker.C:
			logger.Info().Int("attempt", attempt).Msg("gracefully leaving vipcast cluster")
			// It is safe to call Leave() multiple times.
			err := cl.serfInstance.Leave()
			if err != nil {
				// Error on Leave broadcast timeout:
				logger.Err(err).Msg("serf.Leave() failed")
				continue
			}
			return nil
		}
	}
}

func notifyClusterMember(ctx context.Context, method, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusAlreadyReported {
		return nil
	}
	return fmt.Errorf("%s %s returned unexpected status: %d", req.Method, req.URL.String(), resp.StatusCode)
}

func hasNonlocalAddr(clusterPeers []string) bool {
	for _, peer := range clusterPeers {
		if host, _, err := net.SplitHostPort(peer); err == nil {
			peer = host
		}
		if ip := net.ParseIP(peer); ip != nil && !ip.IsLoopback() {
			return true
		} else if ip == nil && strings.ToLower(peer) != "localhost" {
			return true
		}
	}
	return false
}

func isUnspecifiedAddr(addr string) bool {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	return addr == "" || net.ParseIP(addr).IsUnspecified()
}

func isUnroutableAddr(addr string) bool {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	if ip := net.ParseIP(addr); ip != nil && (ip.IsUnspecified() || ip.IsLoopback()) {
		return true // typically 0.0.0.0 or localhost
	} else if ip == nil && strings.ToLower(addr) == "localhost" {
		return true
	}
	return false
}
