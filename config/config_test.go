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

package config

import (
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

func TestDefaultConfig(t *testing.T) {
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(DefaultConfigYaml), &raw); err != nil {
		t.Fatalf("error unmarshalling DefaultConfigYaml: %s", err.Error())
	}
	cfg := DefaultConfig()
	conf := map[string]interface{}{}
	if err := mapstructure.Decode(cfg, &conf); err != nil {
		t.Fatalf("error unmarshalling DefaultConfig(): %s", err.Error())
	}
	if !isEqual(raw, conf) {
		t.Fatalf("DefaultConfigYaml is not equal to Config structure")
	}
}

func isEqual(m1 map[string]interface{}, m2 map[string]interface{}) bool {
	if len(m1) != len(m2) {
		return false
	}
	for key := range m1 {
		v1 := reflect.ValueOf(m1[key])
		v2 := reflect.ValueOf(m2[key])
		if v1.Kind() == reflect.Map && v2.Kind() == reflect.Map {
			return isEqual(m1[key].(map[string]interface{}), m2[key].(map[string]interface{}))
		}
		if v1.Kind() == reflect.Ptr || v1.Kind() == reflect.Interface {
			v1 = v1.Elem()
		}
		if v2.Kind() == reflect.Ptr || v2.Kind() == reflect.Interface {
			v2 = v2.Elem()
		}
		// fmt.Printf("v1.Kind(): %+v\n", v1.Kind())
		// fmt.Printf("v2.Kind(): %+v\n", v2.Kind())
		// fmt.Printf("m1[%s]=%v vs. m2[%s]=%v\n", key, v1, key, v2)
		if v1.Kind() != v2.Kind() {
			return false
		}
		if reflect.DeepEqual(v1, v2) {
			return false
		}
	}
	return true
}
