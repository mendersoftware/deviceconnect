// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		Name string

		DataStoreErr error
		HTTPStatus   int
		HTTPBody     map[string]interface{}
	}{{
		Name:       "ok",
		HTTPStatus: http.StatusNoContent,
	}}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			router := NewRouter()
			req, err := http.NewRequest("GET", "http://localhost"+APIURLHealth, nil)
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)
			assert.Nil(t, w.Body.Bytes())
		})
	}
}
