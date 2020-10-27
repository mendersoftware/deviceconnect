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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mendersoftware/deviceconnect/app"
	app_mocks "github.com/mendersoftware/deviceconnect/app/mocks"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const JWTUser = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibWVuZGVyLnVzZXIiOnRydWUsIm1lbmRlci5wbGFuIjoiZW50ZXJwcmlzZSIsIm1lbmRlci50ZW5hbnQiOiJhYmNkIn0.sn10_eTex-otOTJ7WCp_7NUwiz9lBT0KiPOdZF9Jt4w"
const JWTUserID = "1234567890"
const JWTUserTenantID = "abcd"

func TestManagementGetDevice(t *testing.T) {
	testCases := []struct {
		Name          string
		DeviceID      string
		Authorization string

		GetDevice      *model.Device
		GetDeviceError error

		HTTPStatus int
		Body       *model.Device
	}{
		{
			Name:          "ok",
			DeviceID:      "1234567890",
			Authorization: "Bearer " + JWTUser,

			GetDevice: &model.Device{
				ID:     "1234567890",
				Status: model.DeviceStatusConnected,
			},

			HTTPStatus: 200,
			Body: &model.Device{
				ID:     "1234567890",
				Status: model.DeviceStatusConnected,
			},
		},
		{
			Name:     "ko, missing auth",
			DeviceID: "1234567890",

			HTTPStatus: 401,
		},
		{
			Name:          "ko, not found",
			DeviceID:      "1234567890",
			Authorization: "Bearer " + JWTUser,

			GetDeviceError: app.ErrDeviceNotFound,

			HTTPStatus: 404,
		},
		{
			Name:          "ko, other error",
			DeviceID:      "1234567890",
			Authorization: "Bearer " + JWTUser,

			GetDeviceError: errors.New("error"),

			HTTPStatus: 400,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			app := &app_mocks.App{}
			if tc.Authorization != "" {
				app.On("GetDevice",
					mock.MatchedBy(func(_ context.Context) bool {
						return true
					}),
					JWTUserTenantID,
					tc.DeviceID,
				).Return(tc.GetDevice, tc.GetDeviceError)
			}

			router, _ := NewRouter(app)
			s := httptest.NewServer(router)
			defer s.Close()

			url := strings.Replace(APIURLManagementDevice, ":deviceId", tc.DeviceID, 1)
			req, err := http.NewRequest("GET", "http://localhost"+url, nil)
			if tc.Authorization != "" {
				req.Header.Set(headerAuthorization, tc.Authorization)
			}
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.HTTPStatus, w.Code)

			if tc.HTTPStatus == http.StatusOK {
				var response *model.Device
				body := w.Body.Bytes()
				_ = json.Unmarshal(body, &response)
				assert.Equal(t, tc.Body, response)
			}

			app.AssertExpectations(t)
		})
	}
}

func TestManagementConnect(t *testing.T) {
	testCases := []struct {
		Name          string
		DeviceID      string
		SessionID     string
		Authorization string
	}{
		{
			Name:          "ok",
			DeviceID:      "1234567890",
			SessionID:     "session_id",
			Authorization: "Bearer " + JWTUser,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			app := &app_mocks.App{}
			app.On("SubscribeMessagesFromDevice",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTUserTenantID,
				tc.DeviceID,
			).Return(make(<-chan *model.Message), nil)

			app.On("PrepareUserSession",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTUserTenantID,
				JWTUserID,
				tc.DeviceID,
			).Return(&model.Session{
				ID:       tc.SessionID,
				UserID:   JWTUserID,
				DeviceID: tc.DeviceID,
			}, nil)

			app.On("UpdateUserSessionStatus",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTUserTenantID,
				tc.SessionID,
				model.DeviceStatusConnected,
			).Return(nil)

			app.On("UpdateUserSessionStatus",
				mock.MatchedBy(func(_ context.Context) bool {
					return true
				}),
				JWTUserTenantID,
				tc.SessionID,
				model.DeviceStatusDisconnected,
			).Return(nil)

			router, _ := NewRouter(app)
			s := httptest.NewServer(router)
			defer s.Close()

			url := "ws" + strings.TrimPrefix(s.URL, "http")

			headers := http.Header{}
			headers.Set(headerAuthorization, "Bearer "+JWTUser)

			url = url + strings.Replace(APIURLManagementDeviceConnect, ":deviceId", tc.DeviceID, 1)
			ws, _, err := websocket.DefaultDialer.Dial(url, headers)
			assert.NoError(t, err)

			pingReceived := false
			ws.SetPingHandler(func(message string) error {
				pingReceived = true
				_ = ws.SetReadDeadline(time.Now().Add(time.Duration(pongWait) * time.Second))
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

func TestManagementConnectFailures(t *testing.T) {
	testCases := []struct {
		Name                  string
		DeviceID              string
		SessionID             string
		PrepareUserSessionErr error
		Authorization         string
		HTTPStatus            int
		HTTPError             error
	}{
		{
			Name:          "ko, unable to upgrade",
			SessionID:     "1",
			Authorization: "Bearer " + JWTUser,
			HTTPStatus:    http.StatusBadRequest,
		},
		{
			Name:                  "ko, session preparation failure",
			SessionID:             "1",
			PrepareUserSessionErr: errors.New("Error"),
			Authorization:         "Bearer " + JWTUser,
			HTTPStatus:            http.StatusBadRequest,
		},
		{
			Name:                  "ko, device not found",
			SessionID:             "1",
			PrepareUserSessionErr: app.ErrDeviceNotFound,
			Authorization:         "Bearer " + JWTUser,
			HTTPStatus:            http.StatusNotFound,
		},
		{
			Name:       "ko, missing authorization header",
			HTTPStatus: http.StatusUnauthorized,
			HTTPError:  errors.New("Authorization not present in header"),
		},
		{
			Name:          "ko, malformed authorization header",
			Authorization: "malformed",
			HTTPStatus:    http.StatusUnauthorized,
			HTTPError:     errors.New("malformed Authorization header"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			app := &app_mocks.App{}
			if tc.SessionID != "" {
				app.On("PrepareUserSession",
					mock.MatchedBy(func(_ context.Context) bool {
						return true
					}),
					JWTUserTenantID,
					JWTUserID,
					tc.DeviceID,
				).Return(&model.Session{ID: tc.SessionID}, tc.PrepareUserSessionErr)
			}

			router, _ := NewRouter(app)
			url := strings.Replace(APIURLManagementDeviceConnect, ":deviceId", tc.DeviceID, 1)
			req, err := http.NewRequest("GET", "http://localhost"+url, nil)
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
				_ = json.Unmarshal(body, &response)
				value := response["error"]
				assert.Equal(t, tc.HTTPError.Error(), value)
			}
		})
	}
}
