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

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vmihailenco/msgpack/v5"

	inv_mocks "github.com/mendersoftware/deviceconnect/client/inventory/mocks"
	nats_mocks "github.com/mendersoftware/deviceconnect/client/nats/mocks"
	"github.com/mendersoftware/deviceconnect/model"
	store_mocks "github.com/mendersoftware/deviceconnect/store/mocks"
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/shell"
)

func TestHealthCheck(t *testing.T) {
	err := errors.New("error")

	store := &store_mocks.DataStore{}
	store.On("Ping",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.HealthCheck(ctx)
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestProvisionTenant(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"

	store := &store_mocks.DataStore{}
	store.On("ProvisionTenant",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.ProvisionTenant(ctx, &model.Tenant{TenantID: tenantID})
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestProvisionDevice(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"
	const deviceID = "abcd"

	store := &store_mocks.DataStore{}
	store.On("ProvisionDevice",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		deviceID,
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.ProvisionDevice(ctx, tenantID, &model.Device{ID: deviceID})
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestDeleteDevice(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"
	const deviceID = "abcd"

	store := &store_mocks.DataStore{}
	store.On("DeleteDevice",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		deviceID,
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.DeleteDevice(ctx, tenantID, deviceID)
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestGetDevice(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"
	const deviceID = "abcd"
	device := &model.Device{
		ID: deviceID,
	}

	store := &store_mocks.DataStore{}
	store.On("GetDevice",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		"not-found",
	).Return(nil, nil)

	store.On("GetDevice",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		"error",
	).Return(nil, err)

	store.On("GetDevice",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		deviceID,
	).Return(device, nil)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	_, res := app.GetDevice(ctx, tenantID, "error")
	assert.Equal(t, err, res)

	_, res = app.GetDevice(ctx, tenantID, "not-found")
	assert.Equal(t, ErrDeviceNotFound, res)

	dev, res := app.GetDevice(ctx, tenantID, deviceID)
	assert.NoError(t, res)
	assert.Equal(t, dev, device)

	store.AssertExpectations(t)
}

func TestUpsertDeviceStatus(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"
	const deviceID = "abcd"

	store := &store_mocks.DataStore{}
	store.On("UpsertDeviceStatus",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		deviceID,
		mock.AnythingOfType("string"),
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.UpsertDeviceStatus(ctx, tenantID, deviceID, "anything")
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestPrepareUserSession(t *testing.T) {
	testCases := []struct {
		name             string
		tenantID         string
		userID           string
		deviceID         string
		device           *model.Device
		deviceErr        error
		upsertSessionErr error
		session          *model.Session
		err              error
	}{
		{
			name:      "device error",
			tenantID:  "1",
			userID:    "2",
			deviceID:  "3",
			deviceErr: errors.New("error"),
			err:       errors.New("error"),
		},
		{
			name:     "device not found",
			tenantID: "1",
			userID:   "2",
			deviceID: "3",
			err:      ErrDeviceNotFound,
		},
		{
			name:     "device not connected",
			tenantID: "1",
			userID:   "2",
			deviceID: "3",
			device: &model.Device{
				ID:     "3",
				Status: model.DeviceStatusDisconnected,
			},
			err: ErrDeviceNotConnected,
		},
		{
			name:     "upsert fails",
			tenantID: "1",
			userID:   "2",
			deviceID: "3",
			device: &model.Device{
				ID:     "3",
				Status: model.DeviceStatusConnected,
			},
			upsertSessionErr: errors.New("upsert error"),
			err:              errors.New("upsert error"),
		},
		{
			name:     "upsert fails",
			tenantID: "1",
			userID:   "2",
			deviceID: "3",
			device: &model.Device{
				ID:     "3",
				Status: model.DeviceStatusConnected,
			},
			session: &model.Session{
				ID:       "id",
				UserID:   "2",
				DeviceID: "3",
				Status:   model.SessionStatusDisconnected,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := &store_mocks.DataStore{}
			store.On("GetDevice",
				mock.MatchedBy(func(ctx context.Context) bool {
					return true
				}),
				tc.tenantID,
				tc.deviceID,
			).Return(tc.device, tc.deviceErr)

			if tc.deviceErr == nil && tc.device != nil && tc.device.Status == model.DeviceStatusConnected {
				store.On("UpsertSession",
					mock.MatchedBy(func(ctx context.Context) bool {
						return true
					}),
					tc.tenantID,
					tc.userID,
					tc.deviceID,
				).Return(tc.session, tc.upsertSessionErr)
			}

			app := NewDeviceConnectApp(store, nil, nil)

			ctx := context.Background()
			session, err := app.PrepareUserSession(ctx, tc.tenantID, tc.userID, tc.deviceID)
			assert.Equal(t, tc.session, session)
			assert.Equal(t, tc.err, err)

			store.AssertExpectations(t)
		})
	}
}

func TestUpdateUserSessionStatus(t *testing.T) {
	err := errors.New("error")
	const tenantID = "1234"
	const deviceID = "abcd"

	store := &store_mocks.DataStore{}
	store.On("UpdateSessionStatus",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
		tenantID,
		deviceID,
		mock.AnythingOfType("string"),
	).Return(err)

	app := NewDeviceConnectApp(store, nil, nil)

	ctx := context.Background()
	res := app.UpdateUserSessionStatus(ctx, tenantID, deviceID, "anything")
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

func TestPublishMessageFromDevice(t *testing.T) {
	const tenantID = "abcd"
	const deviceID = "1234567890"

	subject := getMessageSubject(tenantID, deviceID, "device")

	message := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   shell.MessageTypeShellCommand,
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status": "ok",
			},
		},
		Body: []byte("data"),
	}

	client := &nats_mocks.ClientInterface{}
	client.On("Publish",
		subject,
		mock.MatchedBy(func(data []byte) bool {
			decodedMessage := &ws.ProtoMsg{}
			err := msgpack.Unmarshal(data, decodedMessage)
			assert.NoError(t, err)
			assert.Equal(t, message, decodedMessage)

			return true
		}),
	).Return(nil)

	app := NewDeviceConnectApp(nil, client, nil)

	ctx := context.Background()
	err := app.PublishMessageFromDevice(ctx, tenantID, deviceID, message)
	assert.NoError(t, err)
}

func TestPublishMessageFromManagement(t *testing.T) {
	const tenantID = "abcd"
	const deviceID = "1234567890"

	subject := getMessageSubject(tenantID, deviceID, "management")

	message := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   shell.MessageTypeShellCommand,
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status": "ok",
			},
		},
		Body: []byte("data"),
	}

	client := &nats_mocks.ClientInterface{}
	client.On("Publish",
		subject,
		mock.MatchedBy(func(data []byte) bool {
			decodedMessage := &ws.ProtoMsg{}
			err := msgpack.Unmarshal(data, decodedMessage)
			assert.NoError(t, err)
			assert.Equal(t, message, decodedMessage)

			return true
		}),
	).Return(nil)

	app := NewDeviceConnectApp(nil, client, nil)

	ctx := context.Background()
	err := app.PublishMessageFromManagement(ctx, tenantID, deviceID, message)
	assert.NoError(t, err)
}

func TestSubscribeMessagesFromDevice(t *testing.T) {
	const tenantID = "abcd"
	const deviceID = "1234567890"

	subject := getMessageSubject(tenantID, deviceID, "device")

	message := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   shell.MessageTypeShellCommand,
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status": "ok",
			},
		},
		Body: []byte("data"),
	}

	client := &nats_mocks.ClientInterface{}
	client.On("Subscribe",
		subject,
		mock.MatchedBy(func(callback func(msg *nats.Msg)) bool {
			data, err := msgpack.Marshal(message)
			assert.NoError(t, err)
			callback(&nats.Msg{Data: data})

			return true
		}),
	).Return(nil)

	app := NewDeviceConnectApp(nil, client, nil)

	ctx := context.Background()
	out, err := app.SubscribeMessagesFromDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.NotNil(t, out)

	msg := <-out
	assert.Equal(t, message, msg)
}

func TestSubscribeMessagesFromManagement(t *testing.T) {
	const tenantID = "abcd"
	const deviceID = "1234567890"

	subject := getMessageSubject(tenantID, deviceID, "management")

	message := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   shell.MessageTypeShellCommand,
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status": "ok",
			},
		},
		Body: []byte("data"),
	}

	client := &nats_mocks.ClientInterface{}
	client.On("Subscribe",
		subject,
		mock.MatchedBy(func(callback func(msg *nats.Msg)) bool {
			data, err := msgpack.Marshal(message)
			assert.NoError(t, err)
			callback(&nats.Msg{Data: data})

			return true
		}),
	).Return(nil)

	app := NewDeviceConnectApp(nil, client, nil)

	ctx := context.Background()
	out, err := app.SubscribeMessagesFromManagement(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.NotNil(t, out)

	msg := <-out
	assert.Equal(t, message, msg)
}

func TestRemoteTerminalAllowed(t *testing.T) {
	testCases := []struct {
		name                    string
		tenantID                string
		deviceID                string
		groups                  []string
		inventorySearchErr      error
		inventorySearchResCount int

		allowed bool
		err     error
	}{
		{
			name:                    "ok, true",
			tenantID:                "1",
			deviceID:                "2",
			groups:                  []string{"a", "b"},
			inventorySearchResCount: 1,

			allowed: true,
		},
		{
			name:                    "ok, false",
			tenantID:                "1",
			deviceID:                "2",
			groups:                  []string{"a", "b"},
			inventorySearchResCount: 0,

			allowed: false,
		},
		{
			name:                    "ko, inventory error",
			tenantID:                "1",
			deviceID:                "2",
			groups:                  []string{"a", "b"},
			inventorySearchResCount: 0,
			inventorySearchErr:      errors.New("search error"),

			allowed: false,
			err:     errors.New("search error"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inv := &inv_mocks.Client{}
			inv.On("Search",
				mock.MatchedBy(func(ctx context.Context) bool {
					return true
				}),
				tc.tenantID,
				model.SearchParams{
					Page:    1,
					PerPage: 1,
					Filters: []model.FilterPredicate{
						{
							Scope:     model.InventoryGroupScope,
							Attribute: model.InventoryGroupAttributeName,
							Type:      "$in",
							Value:     tc.groups,
						},
					},
					DeviceIDs: []string{tc.deviceID},
				},
			).Return([]model.InvDevice{}, tc.inventorySearchResCount, tc.inventorySearchErr)

			app := NewDeviceConnectApp(nil, nil, inv)

			ctx := context.Background()
			allowed, err := app.RemoteTerminalAllowed(ctx, tc.tenantID, tc.deviceID, tc.groups)
			assert.Equal(t, tc.allowed, allowed)
			assert.Equal(t, tc.err, err)

			inv.AssertExpectations(t)
		})
	}
}
