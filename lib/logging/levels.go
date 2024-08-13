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

// Package logging implements loggong for a vipcast application.

package logging

import (
	"strings"

	"github.com/rs/zerolog"
)

var allowedLogLevels = []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "PANIC", "OFF"}

func AllowedLogLevels() []string {
	var c []string
	copy(c, allowedLogLevels)
	return c
}

func LevelFromString(level string) (bool, zerolog.Level) {
	level = strings.ToLower(strings.TrimSpace(level))
	switch {
	case strings.HasPrefix(level, "off"):
		return true, zerolog.NoLevel
	case strings.HasPrefix(level, "info"):
		level = "info"
	case strings.HasPrefix(level, "warn"):
		level = "warn"
	case strings.HasPrefix(level, "err"):
		level = "error"
	}
	l, err := zerolog.ParseLevel(level)
	if err != nil {
		return false, zerolog.NoLevel
	}
	return true, l
}
