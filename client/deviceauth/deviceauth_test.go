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

package deviceauth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/mendersoftware/deviceconnect/client/deviceauth/mocks"
)

const testURI = "http://localhost:8080"

func TestVerify(t *testing.T) {
	testCases := map[string]struct {
		token  string
		method string
		uri    string

		httpResponse *http.Response
		httpError    error
		result       error
	}{
		"oK": {
			token:  "token",
			method: "GET",
			uri:    "/",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
			},
			result: nil,
		},
		"ko, forbidden": {
			token:  "token",
			method: "GET",
			uri:    "/",
			httpResponse: &http.Response{
				StatusCode: http.StatusForbidden,
			},
			result: ErrForbidden,
		},
		"ko, unauthorized": {
			token:  "token",
			method: "GET",
			uri:    "/",
			httpResponse: &http.Response{
				StatusCode: http.StatusUnauthorized,
			},
			result: ErrUnauthorized,
		},
		"ko, too many requests": {
			token:  "token",
			method: "GET",
			uri:    "/",
			httpResponse: &http.Response{
				StatusCode: http.StatusTooManyRequests,
			},
			result: ErrTooManyRequests,
		},
		"ko, internal server error": {
			token:  "token",
			method: "GET",
			uri:    "/",
			httpResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
			},
			result: ErrInternalError,
		},
		"ko, error": {
			token:     "token",
			method:    "GET",
			uri:       "/",
			httpError: errors.New("error"),
			result:    ErrInternalError,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			mockHTTPClient := &mocks.HTTPClient{}
			mockHTTPClient.On("Do",
				mock.AnythingOfType("*http.Request"),
			).Return(tc.httpResponse, tc.httpError)

			client := NewClient(testURI)
			client.httpClient = mockHTTPClient

			ctx := context.Background()
			res := client.Verify(ctx, tc.token, tc.method, tc.uri)
			assert.Equal(t, tc.result, res)

			mockHTTPClient.AssertExpectations(t)
		})
	}
}

func TestGetHTTPStatusCodeFromError(t *testing.T) {
	testCases := map[string]struct {
		err    error
		status int
	}{
		"forbidden": {
			err:    ErrForbidden,
			status: http.StatusForbidden,
		},
		"unauthorized": {
			err:    ErrUnauthorized,
			status: http.StatusUnauthorized,
		},
		"too many requests": {
			err:    ErrTooManyRequests,
			status: http.StatusTooManyRequests,
		},
		"internal": {
			err:    ErrInternalError,
			status: http.StatusInternalServerError,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			res := GetHTTPStatusCodeFromError(tc.err)
			assert.Equal(t, res, tc.status)
		})
	}
}
