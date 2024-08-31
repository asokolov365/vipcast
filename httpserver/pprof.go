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
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/VictoriaMetrics/metrics"
)

var (
	pprofRequests        = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/"}`)
	pprofCmdlineRequests = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/cmdline"}`)
	pprofProfileRequests = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/profile"}`)
	pprofSymbolRequests  = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/symbol"}`)
	pprofTraceRequests   = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/trace"}`)
	pprofMutexRequests   = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/mutex"}`)
	pprofDefaultRequests = metrics.NewCounter(`vipcast_http_requests_total{path="/debug/pprof/default"}`)
)

func pprofHandler(profileName string, w http.ResponseWriter, r *http.Request) {
	pprofRequests.Inc()
	// This switch has been stolen from init func at https://golang.org/src/net/http/pprof/pprof.go
	switch profileName {
	case "cmdline":
		pprofCmdlineRequests.Inc()
		pprof.Cmdline(w, r)
	case "profile":
		pprofProfileRequests.Inc()
		pprof.Profile(w, r)
	case "symbol":
		pprofSymbolRequests.Inc()
		pprof.Symbol(w, r)
	case "trace":
		pprofTraceRequests.Inc()
		pprof.Trace(w, r)
	case "mutex":
		pprofMutexRequests.Inc()
		seconds, _ := strconv.Atoi(r.FormValue("seconds"))
		if seconds <= 0 {
			seconds = 10
		}
		prev := runtime.SetMutexProfileFraction(10)
		time.Sleep(time.Duration(seconds) * time.Second)
		pprof.Index(w, r)
		runtime.SetMutexProfileFraction(prev)
	default:
		pprofDefaultRequests.Inc()
		pprof.Index(w, r)
	}
}
