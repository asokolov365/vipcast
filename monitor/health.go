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

package monitor

// Health defines possible service health statuses.
type HealthStatus int8

const (
	NotHealthy HealthStatus = iota
	Healthy
	Undefined
)

// String returns a string representation of the Health status
func (t HealthStatus) String() string {
	switch t {
	case NotHealthy:
		return "not healthy"
	case Healthy:
		return "healthy"
	default:
		return "undefined"
	}
}
