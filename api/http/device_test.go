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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	app_mocks "github.com/mendersoftware/deviceconnect/app/mocks"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const JWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibWVuZGVyLmRldmljZSI6dHJ1ZSwibWVuZGVyLnBsYW4iOiJlbnRlcnByaXNlIiwibWVuZGVyLnRlbmFudCI6ImFiY2QifQ.Q4bIDhEx53FLFUMipjJUNgEmEf48yjcaFxlh8XxZFVw"
const JWTDeviceID = "1234567890"
const JWTTenantID = "abcd"

func TestDeviceConnect(t *testing.T) {
	testCases := []struct {
		Name          string
		Authorization string
	}{
		{
			Name:          "ok",
			Authorization: "Bearer " + JWT,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			app := &app_mocks.App{}
			app.On("UpdateDeviceStatus",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTTenantID,
				JWTDeviceID,
				model.DeviceStatusConnected,
			).Return(nil)

			app.On("UpdateDeviceStatus",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTTenantID,
				JWTDeviceID,
				model.DeviceStatusDisconnected,
			).Return(nil)

			router, _ := NewRouter(app)
			s := httptest.NewServer(router)
			defer s.Close()

			url := "ws" + strings.TrimPrefix(s.URL, "http")

			headers := http.Header{}
			headers.Set(headerAuthorization, "Bearer "+JWT)

			ws, _, err := websocket.DefaultDialer.Dial(url+APIURLDevicesConnect, headers)
			assert.NoError(t, err)

			pingReceived := false
			ws.SetPingHandler(func(message string) error {
				pingReceived = true
				ws.SetReadDeadline(time.Now().Add(time.Duration(pongWait) * time.Second))
				return ws.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(writeWait))
			})

			go func() {
				for {
					_, _, err := ws.ReadMessage()
					if err != nil {
						break
					}
				}
			}()

			// wait 1s to let the first ping flow in
			time.Sleep(1 * time.Second)
			assert.True(t, pingReceived)

			// close the websocket
			ws.Close()

			// wait 100ms to let the websocket fully shutdown on the server
			time.Sleep(100 * time.Millisecond)

			app.AssertExpectations(t)
		})
	}
}

func TestDeviceConnectFailures(t *testing.T) {
	testCases := []struct {
		Name          string
		Authorization string
		HTTPStatus    int
		HTTPError     error
	}{
		{
			Name:          "ko, unable to upgrade",
			Authorization: "Bearer " + JWT,
			HTTPStatus:    http.StatusBadRequest,
		},
		{
			Name:       "ko, missing authorization header",
			HTTPStatus: http.StatusBadRequest,
			HTTPError:  ErrMissingAuthentication,
		},
		{
			Name:          "ko, malformed authorization header",
			Authorization: "malformed",
			HTTPStatus:    http.StatusBadRequest,
			HTTPError:     ErrMissingAuthentication,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			router, _ := NewRouter(nil)
			req, err := http.NewRequest("GET", "http://localhost"+APIURLDevicesConnect, nil)
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			if tc.Authorization != "" {
				req.Header.Add("Authorization", tc.Authorization)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)

			if tc.HTTPError != nil {
				var response map[string]string
				body := w.Body.Bytes()
				err = json.Unmarshal(body, &response)
				value, _ := response["error"]
				assert.Equal(t, tc.HTTPError.Error(), value)
			}
		})
	}
}

func TestProvisionDevice(t *testing.T) {
	testCases := []struct {
		Name               string
		TenantID           string
		DeviceID           string
		Device             string
		ProvisionDeviceErr error
		HTTPStatus         int
	}{
		{
			Name:       "ok",
			TenantID:   "1234",
			DeviceID:   "1234",
			Device:     `{"device_id": "1234"}`,
			HTTPStatus: http.StatusCreated,
		},
		{
			Name:       "ko, empty payload",
			TenantID:   "1234",
			Device:     ``,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:       "ko, bad payload",
			TenantID:   "1234",
			Device:     `...`,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:       "ko, empty device ID",
			TenantID:   "1234",
			Device:     `{"device_id": ""}`,
			HTTPStatus: http.StatusBadRequest,
		},
		{
			Name:               "ko, error",
			TenantID:           "1234",
			DeviceID:           "1234",
			Device:             `{"device_id": "1234"}`,
			ProvisionDeviceErr: errors.New("error"),
			HTTPStatus:         http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			deviceConnectApp := &app_mocks.App{}
			if tc.DeviceID != "" {
				deviceConnectApp.On("ProvisionDevice",
					mock.MatchedBy(func(_ context.Context) bool {
						return true
					}),
					tc.TenantID,
					&model.Device{ID: tc.DeviceID},
				).Return(tc.ProvisionDeviceErr)
			}

			router, _ := NewRouter(deviceConnectApp)

			url := strings.Replace(APIURLInternalDevices, ":tenantId", tc.TenantID, 1)
			req, err := http.NewRequest("POST", url, strings.NewReader(tc.Device))
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)

			deviceConnectApp.AssertExpectations(t)
		})
	}
}

func TestDeleteDevice(t *testing.T) {
	testCases := []struct {
		Name               string
		TenantID           string
		DeviceID           string
		ProvisionDeviceErr error
		HTTPStatus         int
	}{
		{
			Name:       "ok",
			TenantID:   "1234",
			DeviceID:   "abcd",
			HTTPStatus: http.StatusAccepted,
		},
		{
			Name:               "ko, empty device id",
			TenantID:           "1234",
			ProvisionDeviceErr: errors.New("error"),
			HTTPStatus:         http.StatusNotFound,
		},
		{
			Name:               "ko, error",
			TenantID:           "1234",
			DeviceID:           "abcd",
			ProvisionDeviceErr: errors.New("error"),
			HTTPStatus:         http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			deviceConnectApp := &app_mocks.App{}
			if tc.DeviceID != "" {
				deviceConnectApp.On("DeleteDevice",
					mock.MatchedBy(func(_ context.Context) bool {
						return true
					}),
					tc.TenantID,
					tc.DeviceID,
				).Return(tc.ProvisionDeviceErr)
			}

			router, _ := NewRouter(deviceConnectApp)

			url := strings.Replace(APIURLInternalDevicesID, ":tenantId", tc.TenantID, 1)
			url = strings.Replace(url, ":deviceId", tc.DeviceID, 1)
			req, err := http.NewRequest("DELETE", url, nil)
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)

			deviceConnectApp.AssertExpectations(t)
		})
	}
}
