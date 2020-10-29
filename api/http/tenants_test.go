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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	app_mocks "github.com/mendersoftware/deviceconnect/app/mocks"
	"github.com/mendersoftware/deviceconnect/model"
)

func TestProvision(t *testing.T) {
	testCases := []struct {
		Name               string
		TenantID           string
		Tenant             string
		ProvisionTenantErr error
		HTTPStatus         int
	}{
		{
			Name:       "ok",
			TenantID:   "1234",
			Tenant:     `{"tenant_id": "1234"}`,
			HTTPStatus: http.StatusCreated,
		},
		{
			Name:       "ko, empty payload",
			Tenant:     ``,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:       "ko, bad payload",
			Tenant:     `...`,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:       "ko, empty tenant ID",
			Tenant:     `{"tenant_id": ""}`,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:               "ko, error",
			TenantID:           "1234",
			Tenant:             `{"tenant_id": "1234"}`,
			ProvisionTenantErr: errors.New("error"),
			HTTPStatus:         http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			deviceConnectApp := &app_mocks.App{}
			if tc.TenantID != "" {
				deviceConnectApp.On("ProvisionTenant",
					mock.MatchedBy(func(_ context.Context) bool {
						return true
					}),
					&model.Tenant{TenantID: tc.TenantID},
				).Return(tc.ProvisionTenantErr)
			}

			router, _ := NewRouter(deviceConnectApp)

			req, err := http.NewRequest("POST", APIURLInternalTenants, strings.NewReader(tc.Tenant))
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)
			if tc.HTTPStatus == http.StatusNoContent {
				assert.Nil(t, w.Body.Bytes())
			}

			deviceConnectApp.AssertExpectations(t)
		})
	}
}
