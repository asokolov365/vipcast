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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/monitor"
	"github.com/asokolov365/vipcast/registry"
	"github.com/asokolov365/vipcast/route"
	"github.com/rs/zerolog"
)

var (
	apiV1Requests = metrics.NewCounter(`vipcast_http_requests_total{path="/api/v1/"}`)
)

func apiV1Handler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/" {
		if r.Method != http.MethodGet {
			return false
		}
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h2>vipcast</h2>")
		fmt.Fprintf(w, "See docs at <a href='https://github.com/asokolov365/vipcast/blob/main/README.md'>https://github.com/asokolov365/vipcast</a></br>")
		fmt.Fprintf(w, "Useful endpoints:</br>")
		WriteAPIHelp(w, [][2]string{
			{enum.RegistryApiPath, "advanced information about discovered VIPs in JSON format"},
			{enum.MaintenanceApiPath, "maintenance status for discovered active VIPs"},
			{"/metrics", "available service metrics"},
		})
		return true
	}

	path := strings.Replace(r.URL.Path, "//", "/", -1)

	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}

	apiV1Requests.Inc()

	switch path {
	case enum.RegistryApiPath:
		if err := apiV1Registry(w, r); err != nil {
			Errorf(w, r, "%s", err)
			return true
		}
		return true
	case enum.MaintenanceApiPath:
		if err := apiV1Maintenance(w, r); err != nil {
			Errorf(w, r, "%s", err)
			return true
		}
		return true

	default:
		return false
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
	// vipAddress and monitorString are mandatory
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
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Error().
			Int("status", http.StatusBadRequest).
			Str("error", "Bad Request").
			Msg("vipcast_monitor=consul is not supported, please use consul tags instead")
		return
	default:
		m, err = monitor.NewNoneMonitor(serviceName, vipAddress, bgpCommString, enum.AdminRegistrar)
	}
	if err != nil {
		log.Err(err).Send()
		w.WriteHeader(http.StatusBadRequest)
		httpLog.Err(err).Int("status", http.StatusBadRequest).Send()
		return
	}
	monitor.Registry().AddMonitor(m)
	w.WriteHeader(http.StatusOK)
	httpLog.Info().Int("status", http.StatusOK).
		Str("service", m.Service()).
		Str("vip", m.VipAddress()).
		Msg("adding new service to vipcast")
}

func apiV1Unregister(w http.ResponseWriter, r *http.Request) {
	log := logger.With().Str("path", "/unregister").Logger()
	httpLog := zerolog.New(w)
	var vipAddress string
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

	if err := monitor.Registry().RemoveMonitor(vipAddress); err != nil {
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

func apiV1Registry(w http.ResponseWriter, r *http.Request) error {
	var vipAddress string
	log := logger.With().Str("path", enum.RegistryApiPath).Logger()
	vipAddress = strings.TrimSpace(r.URL.Query().Get("vip"))

	if vipAddress != "" {
		n, err := route.ParseVIP(vipAddress)
		if err != nil {
			return err
		}
		vipAddress = n.String()
	}

	switch r.Method {
	case http.MethodGet, "": // an empty string means GET
		var body []byte
		var err error
		if vipAddress == "" {
			if registry.Registry().Len() == 0 {
				body = []byte("{}")
				w.WriteHeader(http.StatusNoContent)
			} else {
				body, err = registry.Registry().AsJSON()
				if err != nil {
					return err
				}
			}

		} else {
			v := registry.Registry().GetVipInfo(vipAddress)
			if v == nil {
				body = []byte("{}")
				w.WriteHeader(http.StatusNotFound)
			} else {
				body, err = json.Marshal(v)
				if err != nil {
					return err
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
		return nil

	case http.MethodPut:
		vdb := &registry.VipDatabase{}
		err := decodeJSONBody(w, r, vdb)
		if err != nil {
			return err
		}
		for _, v := range vdb.Data {
			updated, _ := registry.Registry().NotifyVipInfoChange(v)

			if updated {
				log.Info().Str("vip", v.VipAddress).
					Strs("reporters", v.Reporters).
					Bool("maintenance", v.IsUnderMaintenance).
					Msg("vip info have been updated")
			} else {
				log.Debug().Str("vip", v.VipAddress).
					Msg("vip info is already in sync")
				delete(vdb.Data, v.VipAddress)
			}
		}
		body, err := json.MarshalIndent(vdb, "", "  ")
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		if len(vdb.Data) == 0 {
			w.WriteHeader(http.StatusAlreadyReported)
		} else {
			w.WriteHeader(http.StatusAccepted)
		}
		w.Write(body)
		return nil

	default:
		return fmt.Errorf("unsupported method: %s", r.Method)
	}
}

func apiV1Maintenance(w http.ResponseWriter, r *http.Request) error {
	log := logger.With().Str("path", enum.MaintenanceApiPath).Logger()

	switch r.Method {
	case http.MethodGet, "": // an empty string means GET
		return apiV1Registry(w, r)

	case http.MethodPost:
		v := &registry.VipInfo{}
		err := decodeJSONBody(w, r, v)
		if err != nil {
			return err
		}
		registry.Registry().SetVipMaintenance(v.VipAddress, v.IsUnderMaintenance)
		log.Info().Str("vip", v.VipAddress).Bool("maintenance", v.IsUnderMaintenance).
			Msg("maintenance set")

		v = registry.Registry().GetVipInfo(v.VipAddress)
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write(b)
		return nil

	default:
		return fmt.Errorf("unsupported method: %s", r.Method)
	}
}
