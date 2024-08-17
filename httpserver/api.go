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

package httpserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/rs/zerolog"
)

func apiV1Handler(endPoint string, w http.ResponseWriter, r *http.Request) {
	switch endPoint {
	case "register":
		apiV1Register(w, r)
	case "unregister":
		apiV1Unregister(w, r)
	case "maintenance":
		apiV1Maintenance(w, r)
	}
}

func apiV1Register(w http.ResponseWriter, r *http.Request) {
	log := logger.With().Str("path", "/register").Logger()
	httpLog := zerolog.New(w)
	var (
		serviceName   string
		vipAddress    string
		bgpCommString string
		monitorString string
	)
	params := r.URL.Query()
	for k, values := range params {
		switch k {
		case "name", "service":
			serviceName = values[0]
		case "vip", "vipcast_vip", "gocast_vip":
			vipAddress = values[0]
		case "communities", "bgp_communities", "vipcast_bgp_communities", "gocast_vip_communities":
			bgpCommString = values[0]
		case "monitor", "vipcast_monitor", "gocast_monitor":
			monitorString = values[0]
		}
	}
	// VipAddress and monitor monitorString are mandatory
	if strings.TrimSpace(vipAddress) == "" {
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("missing required parameter: 'vipcast_vip'")
		return
	}

	if strings.TrimSpace(monitorString) == "" {
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("missing required parameter: 'vipcast_monitor'")
		return
	}

	if strings.TrimSpace(serviceName) == "" {
		serviceName = fmt.Sprintf("undefined-%s", vipAddress)
	}

	var m *monitor.Monitor
	var err error
	switch {
	case strings.TrimSpace(monitorString) == "consul":
		m, err = monitor.NewConsulMonitor(serviceName, vipAddress, bgpCommString, enum.AdminRegistrar)
	default:
		m, err = monitor.NewNoneMonitor(serviceName, vipAddress, bgpCommString, enum.AdminRegistrar)
	}
	if err != nil {
		log.Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Err(err).Int("status", http.StatusBadRequest).Send()
		return
	}
	monitor.Storage().AddMonitor(m)
	w.WriteHeader(http.StatusOK)
	httpLog.Info().Int("status", http.StatusOK).
		Str("service", m.Service()).
		Str("vip", m.VipAddress()).
		Msg("adding new service to vipcast")
}

func apiV1Unregister(w http.ResponseWriter, r *http.Request) {
	log := logger.With().Str("path", "/unregister").Logger()
	httpLog := zerolog.New(w)
	var (
		vipAddress string
	)
	params := r.URL.Query()
	for k, values := range params {
		switch k {
		case "vip", "vipcast_vip", "gocast_vip":
			vipAddress = values[0]
		}
	}
	// Only VipAddress is mandatory
	if strings.TrimSpace(vipAddress) == "" {
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("missing required parameter: 'vipcast_vip'")
		return
	}

	if err := monitor.Storage().RemoveMonitor(vipAddress); err != nil {
		log.Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Err(err).Int("status", http.StatusBadRequest).Send()
		return
	}
	w.WriteHeader(http.StatusOK)
	httpLog.Info().Int("status", http.StatusOK).
		Str("vip", vipAddress).
		Msg("removing service from vipcast")
}

func apiV1Maintenance(w http.ResponseWriter, r *http.Request) {
	log := logger.With().Str("path", "/maintenance").Logger()
	httpLog := zerolog.New(w)
	var (
		vipAddress string
		mode       enum.MaintenanceMode = 2
	)
	params := r.URL.Query()
	for k, values := range params {
		switch k {
		case "vip", "vipcast_vip", "gocast_vip":
			vipAddress = values[0]
		case "on", "yes", "true", "enable":
			mode = enum.MaintenanceOn
		case "off", "no", "false", "disable":
			mode = enum.MaintenanceOff
		}
	}
	// VipAddress is mandatory
	if strings.TrimSpace(vipAddress) == "" {
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("missing required parameter: 'vipcast_vip'")
		return
	}

	if mode == enum.MaintenanceUndefined {
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("missing required parameter: 'on/off'")
		return
	}

	if err := monitor.Storage().SetMaintenance(vipAddress, mode); err != nil {
		log.Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Err(err).Int("status", http.StatusBadRequest).Send()
		return
	}
	w.WriteHeader(http.StatusOK)
	httpLog.Info().Int("status", http.StatusOK).
		Str("vip", vipAddress).
		Msgf("maintenance mode %s", mode.String())
}
