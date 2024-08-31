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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" {
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
		if mediaType != "application/json" {
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("Content-Type header is not application/json"),
				StatusCode: http.StatusUnsupportedMediaType,
			}
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(&dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxError):
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body contains badly-formed JSON (at position %d)", syntaxError.Offset),
				StatusCode: http.StatusBadRequest,
			}

		case errors.Is(err, io.ErrUnexpectedEOF):
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body contains badly-formed JSON"),
				StatusCode: http.StatusBadRequest,
			}

		case errors.As(err, &unmarshalTypeError):
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body contains an invalid value for the %q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset),
				StatusCode: http.StatusBadRequest,
			}

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body contains unknown field %s", fieldName),
				StatusCode: http.StatusBadRequest,
			}

		case errors.Is(err, io.EOF):
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body must not be empty"),
				StatusCode: http.StatusBadRequest,
			}

		case err.Error() == "http: request body too large":
			return &ErrorWithStatusCode{
				Err:        fmt.Errorf("request body must not be larger than 1MB"),
				StatusCode: http.StatusRequestEntityTooLarge,
			}

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return &ErrorWithStatusCode{
			Err:        fmt.Errorf("request body must only contain a single JSON object"),
			StatusCode: http.StatusBadRequest,
		}
	}

	return nil
}
