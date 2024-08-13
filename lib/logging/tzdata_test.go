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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetTimezoneInvalidTimezone(t *testing.T) {
	tz := "foobar"
	_, err := time.LoadLocation(tz)
	require.ErrorContains(t, err, "unknown time zone foobar")
}

func TestGetTimezoneValidTimezone(t *testing.T) {
	tz := "America/Los_Angeles"
	location, err := time.LoadLocation(tz)
	require.NoError(t, err)
	require.NotNil(t, location)
}
