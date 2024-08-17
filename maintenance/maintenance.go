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

package maintenance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/enum"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/rs/zerolog"
)

var logger *zerolog.Logger

var mntDB *Maintenance
var mntLock sync.Mutex

type Maintenance struct {
	lock sync.Mutex
	db   map[string]bool
}

// Storage returns the storage for all known monitors.
func MntDB() *Maintenance {
	if mntDB == nil {
		if logger == nil {
			logger = logging.GetSubLogger("maintenance")
		}
		mntLock.Lock()
		mntDB = &Maintenance{
			lock: sync.Mutex{},
			db:   make(map[string]bool, 100),
		}
		mntLock.Unlock()
	}
	return mntDB
}

// SetMaintenance sets the maintenance flag for the VIP in global Maintenance Database
func (m *Maintenance) SetMaintenance(vip string, mode enum.MaintenanceMode) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.db[vip] = (mode == enum.MaintenanceOn)

	// var err error
	f, err := createOutputFile(*config.AppConfig.StateFile)
	defer f.Close()
	if err != nil {
		logger.Err(err).Send()
		return
	}
	jsonStr, err := json.Marshal(m.db)
	// err = json.NewEncoder(f).Encode(m.db)
	if err != nil {
		logger.Err(err).Send()
		return
	}
	f.Write(jsonStr)
}

func createOutputFile(path string) (*os.File, error) {
	fileDir := filepath.Dir(path)

	if err := os.MkdirAll(fileDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to make directory %q: %w", fileDir, err)
	}
	os.Remove(path)
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644) //nolint
}
