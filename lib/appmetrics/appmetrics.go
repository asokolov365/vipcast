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

package appmetrics

import (
	"fmt"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/lib/bytesutil"
	"github.com/asokolov365/vipcast/version"
)

var versionRe = regexp.MustCompile(`v\d+\.\d+\.\d+`)

// WritePrometheusMetrics writes all the registered metrics to w in Prometheus format.
func WritePrometheusMetrics(w io.Writer) {
	currentTime := time.Now()
	metricsCacheLock.Lock()
	if currentTime.Sub(metricsCacheLastUpdateTime) > time.Second {
		var bb bytesutil.ByteBuffer
		writePrometheusMetrics(&bb)
		metricsCache.Store(&bb)
		metricsCacheLastUpdateTime = currentTime
	}
	metricsCacheLock.Unlock()

	bb := metricsCache.Load()
	_, _ = w.Write(bb.B)
}

var (
	metricsCacheLock           sync.Mutex
	metricsCacheLastUpdateTime time.Time
	metricsCache               atomic.Pointer[bytesutil.ByteBuffer]
)

func writePrometheusMetrics(w io.Writer) {
	metrics.WritePrometheus(w, true)
	// metrics.WriteFDMetrics(w)
	short_version := versionRe.FindString(version.Version)
	if len(short_version) == 0 {
		short_version = "0.0.0"
	}
	fmt.Fprintf(w, "vipcast_version{version=%q, short_version=%q} 1\n", version.Version, short_version)
	// Export start time and uptime in seconds
	fmt.Fprintf(w, "vipcast_app_start_timestamp %d\n", startTime.Unix())
	fmt.Fprintf(w, "vipcast_app_uptime_seconds %d\n", int(time.Since(startTime).Seconds()))
}

var startTime = time.Now()
