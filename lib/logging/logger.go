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

package logging

import (
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/config"
	"github.com/rs/zerolog"
)

const LogMessagesCounterMetricName = "vipcast_log_messages_total"

var (
	loggers     = make(map[string]*zerolog.Logger)
	loggersLock sync.Mutex
)

// Init sets up logging for the application.
func Init(config *config.LoggingConfig, output io.Writer) error {
	zerolog.LevelFieldMarshalFunc = func(lvl zerolog.Level) string {
		_, file, line, ok := runtime.Caller(3)
		if !ok {
			file = "*unknown*"
			line = 0
		}
		file = trimCallerFilePrefix(file)
		callerLocation := fmt.Sprintf(`%s:%d`, file, line)
		labels := fmt.Sprintf(`level=%q, caller=%q`, lvl.String(), callerLocation)
		// Increment log_messages_total Counter
		counterName := fmt.Sprintf(`%s{%s}`, LogMessagesCounterMetricName, labels)
		metrics.GetOrCreateCounter(counterName).Inc()
		return lvl.String()
	}

	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		file = trimCallerFilePrefix(file)
		return file + ":" + strconv.Itoa(line)
	}

	logger := zerolog.New(
		// zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339},
		&NoErrorWriter{writer: output},
	)

	ok, minLevel := LevelFromString(*config.Level)
	if !ok {
		return fmt.Errorf("invalid log level: %s. Valid log levels are: %v",
			config.Level,
			allowedLogLevels,
		)
	}
	zerolog.SetGlobalLevel(minLevel)
	logger = logger.Level(minLevel)

	if *config.Timestamps {
		logger = logger.With().Timestamp().Logger()
	}

	if *config.IncludeLocation {
		logger = logger.With().Caller().Logger()
	}

	levelSampler := zerolog.LevelSampler{}

	if *config.LogLimits.TracePerSecondLimit > 0 {
		levelSampler.TraceSampler = &zerolog.BurstSampler{
			Burst:  *config.LogLimits.TracePerSecondLimit,
			Period: 1 * time.Second,
		}
	}
	if *config.LogLimits.DebugPerSecondLimit > 0 {
		levelSampler.DebugSampler = &zerolog.BurstSampler{
			Burst:  *config.LogLimits.DebugPerSecondLimit,
			Period: 1 * time.Second,
		}
	}
	if *config.LogLimits.InfoPerSecondLimit > 0 {
		levelSampler.InfoSampler = &zerolog.BurstSampler{
			Burst:  *config.LogLimits.InfoPerSecondLimit,
			Period: 1 * time.Second,
		}
	}
	if *config.LogLimits.WarnsPerSecondLimit > 0 {
		levelSampler.WarnSampler = &zerolog.BurstSampler{
			Burst:  *config.LogLimits.WarnsPerSecondLimit,
			Period: 1 * time.Second,
		}
	}
	if *config.LogLimits.ErrorsPerSecondLimit > 0 {
		levelSampler.ErrorSampler = &zerolog.BurstSampler{
			Burst:  *config.LogLimits.ErrorsPerSecondLimit,
			Period: 1 * time.Second,
		}
	}
	logger = logger.Sample(levelSampler)

	loggersLock.Lock()
	loggers["root"] = &logger
	loggersLock.Unlock()
	return nil
}

func GetSubLogger(subsystem string) *zerolog.Logger {
	if _, exists := loggers[subsystem]; !exists {
		l := loggers["root"].With().Str("subsystem", subsystem).Logger()
		loggersLock.Lock()
		loggers[subsystem] = &l
		loggersLock.Unlock()
	}
	return loggers[subsystem]
}

func trimCallerFilePrefix(file string) string {
	if n := strings.Index(file, "/asokolov365/vipcast/"); n >= 0 {
		// Strip */asokolov365/vipcast/ prefix
		file = file[n+len("/asokolov365/vipcast/"):]
	}
	return file
}
