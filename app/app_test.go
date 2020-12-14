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
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	inv_mocks "github.com/mendersoftware/deviceconnect/client/inventory/mocks"
	"github.com/mendersoftware/deviceconnect/client/workflows"
	wf_mocks "github.com/mendersoftware/deviceconnect/client/workflows/mocks"
	"github.com/mendersoftware/deviceconnect/model"
	store_mocks "github.com/mendersoftware/deviceconnect/store/mocks"
)

func TestHealthCheck(t *testing.T) {
	err := errors.New("error")

	store := &store_mocks.DataStore{}
	store.On("Ping",
		mock.MatchedBy(func(ctx context.Context) bool {
			return true
		}),
	).Return(err)

	app := New(store, nil, nil)

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

	app := New(store, nil, nil)

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

	app := New(store, nil, nil)

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

	app := New(store, nil, nil)

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

	app := New(store, nil, nil)

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

func TestUpdateDeviceStatus(t *testing.T) {
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

	app := New(store, nil, nil)

	ctx := context.Background()
	res := app.UpdateDeviceStatus(ctx, tenantID, deviceID, "anything")
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}

type brokenReader struct{}

func (r brokenReader) Read(b []byte) (int, error) {
	return 0, errors.New("broken reader")
}

func TestPrepareUserSession(t *testing.T) {
	testCases := []struct {
		Name string

		CTX     context.Context
		Session *model.Session

		Rand          io.Reader
		BadParameters bool

		StoreGetDevice    *model.Device
		StoreGetDeviceErr error

		StoreAllocSessErr error

		HaveAuditLogs         bool
		WorkflowsError        error
		StoreDeleteSessionErr error

		Erre error
	}{{
		Name: "ok",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusConnected,
		},
		StoreGetDeviceErr: nil,
		StoreAllocSessErr: nil,

		WorkflowsError: nil,
	}, {
		Name: "ok, with auditlogs",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusConnected,
		},
		StoreGetDeviceErr: nil,
		StoreAllocSessErr: nil,

		HaveAuditLogs:  true,
		WorkflowsError: nil,
	}, {
		Name: "error, nil session",

		CTX:           context.Background(),
		BadParameters: true,

		Erre: errors.New("nil Session"),
	}, {
		Name: "error, RNG malfunction",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		BadParameters: true,

		Rand: brokenReader{},

		Erre: errors.New("^failed to generate session ID: broken reader$"),
	}, {
		Name: "error, RNG malfunction",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
		},
		BadParameters: true,

		Erre: errors.New("^app: cannot create invalid Session: " +
			"start_ts: cannot be blank.$"),
	}, {
		Name: "error, GetDevice internal error",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},

		StoreGetDeviceErr: errors.New("store: internal error"),

		Erre: errors.New("^store: internal error$"),
	}, {
		Name: "error, GetDevice not found",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},

		Erre: ErrDeviceNotFound,
	}, {
		Name: "error, GetDevice disconnected",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusDisconnected,
		},
		Erre: errors.New("device not connected"),
	}, {
		Name: "error, AllocateSession internal error",

		CTX: context.Background(),
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusConnected,
		},
		StoreAllocSessErr: errors.New("store: internal error"),
		Erre:              errors.New("store: internal error"),
	}, {
		Name: "error, SubmitAuditLog http error",

		CTX:           context.Background(),
		HaveAuditLogs: true,
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusConnected,
		},
		WorkflowsError: errors.New("http error"),
		Erre: errors.New(
			"failed to submit audit log for creating terminal " +
				"session: http error",
		),
	}, {
		Name: "error, SubmitAuditLog http error and cleanup error",

		CTX:           context.Background(),
		HaveAuditLogs: true,
		Session: &model.Session{
			DeviceID: "00000000-0000-0000-0000-000000000000",
			UserID:   "00000000-0000-0000-0000-000000000001",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now(),
		},
		StoreGetDevice: &model.Device{
			ID:     "00000000-0000-0000-0000-000000000000",
			Status: model.DeviceStatusConnected,
		},
		WorkflowsError:        errors.New("http error"),
		StoreDeleteSessionErr: errors.New("store: internal error"),
		Erre: errors.New(
			"failed to submit audit log for creating terminal " +
				"session: http error: failed to clean up " +
				"session state: store: internal error",
		),
	}}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			ds := new(store_mocks.DataStore)
			defer ds.AssertExpectations(t)
			wf := new(wf_mocks.Client)
			defer wf.AssertExpectations(t)
			inv := new(inv_mocks.Client)
			defer inv.AssertExpectations(t)
			uuid.SetRand(tc.Rand)
			defer uuid.SetRand(nil)
			app := New(
				ds, inv,
				wf, Config{HaveAuditLogs: tc.HaveAuditLogs},
			)
			if tc.BadParameters {
				goto execTest
			}
			ds.On("GetDevice",
				tc.CTX,
				tc.Session.TenantID,
				tc.Session.DeviceID).
				Return(tc.StoreGetDevice, tc.StoreGetDeviceErr)
			if tc.StoreGetDeviceErr != nil ||
				(tc.StoreGetDeviceErr == nil &&
					tc.StoreGetDevice == nil) ||
				tc.StoreGetDevice.Status !=
					model.DeviceStatusConnected {
				goto execTest
			}
			ds.On("AllocateSession", tc.CTX, tc.Session).
				Return(tc.StoreAllocSessErr)
			if tc.StoreAllocSessErr != nil {
				goto execTest
			}
			if !tc.HaveAuditLogs {
				goto execTest
			}
			wf.On("SubmitAuditLog",
				tc.CTX,
				mock.AnythingOfType("workflows.AuditLog")).
				Return(tc.WorkflowsError)
			if tc.WorkflowsError == nil {
				goto execTest
			}
			ds.On("DeleteSession",
				tc.CTX,
				mock.AnythingOfType("string")).
				Return(tc.Session, tc.StoreDeleteSessionErr)

		execTest:
			err := app.PrepareUserSession(tc.CTX, tc.Session)
			if tc.Erre != nil {
				if assert.Error(t, err) {
					assert.Regexp(t,
						tc.Erre.Error(),
						err.Error(),
					)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFreeUserSession(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		SessionID string

		StoreDeleteSession    *model.Session
		StoreDeleteSessionErr error

		HaveAuditLogs bool
		WorkflowsErr  error

		Erre error
	}{{
		Name: "ok",

		SessionID: "00000000-0000-0000-0000-000000000000",

		StoreDeleteSession: &model.Session{
			ID:       "00000000-0000-0000-0000-000000000000",
			DeviceID: "00000000-0000-0000-0000-000000000001",
			UserID:   "00000000-0000-0000-0000-000000000002",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now().Add(-time.Hour),
		},
	}, {
		Name: "ok, with audit logs",

		SessionID: "00000000-0000-0000-0000-000000000000",

		StoreDeleteSession: &model.Session{
			ID:       "00000000-0000-0000-0000-000000000000",
			DeviceID: "00000000-0000-0000-0000-000000000001",
			UserID:   "00000000-0000-0000-0000-000000000002",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now().Add(-time.Hour),
		},
		HaveAuditLogs: true,
	}, {
		Name: "error, store.DeleteSession internal error",

		SessionID: "00000000-0000-0000-0000-000000000000",

		HaveAuditLogs:         true,
		StoreDeleteSessionErr: errors.New("store: internal error"),

		Erre: errors.New("store: internal error$"),
	}, {
		Name: "error, SubmitAuditLogs http error",

		SessionID: "00000000-0000-0000-0000-000000000000",

		StoreDeleteSession: &model.Session{
			ID:       "00000000-0000-0000-0000-000000000000",
			DeviceID: "00000000-0000-0000-0000-000000000001",
			UserID:   "00000000-0000-0000-0000-000000000002",
			TenantID: "000000000000000000000000",
			StartTS:  time.Now().Add(-time.Hour),
		},
		HaveAuditLogs: true,
		WorkflowsErr:  errors.New("http error"),

		Erre: errors.New("http error$"),
	}}

	workflowsMatcher := func(sess *model.Session) func(workflows.AuditLog) bool {
		return func(wf workflows.AuditLog) bool {
			return true
		}
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			ds := new(store_mocks.DataStore)
			wf := new(wf_mocks.Client)
			defer ds.AssertExpectations(t)
			defer wf.AssertExpectations(t)
			app := New(ds, nil, wf, Config{HaveAuditLogs: tc.HaveAuditLogs})
			ctx := context.Background()

			ds.On("DeleteSession", ctx, tc.SessionID).
				Return(tc.StoreDeleteSession, tc.StoreDeleteSessionErr)
			if tc.StoreDeleteSessionErr != nil || !tc.HaveAuditLogs {
				goto execTest
			}
			wf.On("SubmitAuditLog", ctx,
				mock.MatchedBy(workflowsMatcher(tc.StoreDeleteSession))).
				Return(tc.WorkflowsErr)

		execTest:
			err := app.FreeUserSession(ctx, tc.SessionID)
			if tc.Erre != nil {
				if assert.Error(t, err) {
					assert.Regexp(t,
						tc.Erre.Error(),
						err.Error(),
					)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
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

			app := New(nil, inv, nil)

			ctx := context.Background()
			allowed, err := app.RemoteTerminalAllowed(ctx, tc.tenantID, tc.deviceID, tc.groups)
			assert.Equal(t, tc.allowed, allowed)
			assert.Equal(t, tc.err, err)

			inv.AssertExpectations(t)
		})
	}
}
