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
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/mendersoftware/deviceconnect/client/inventory"
	"github.com/mendersoftware/deviceconnect/client/workflows"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/deviceconnect/store"
)

// App errors
var (
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceNotConnected = errors.New("device not connected")
)

// App interface describes app objects
//nolint:lll
//go:generate ../utils/mockgen.sh
type App interface {
	HealthCheck(ctx context.Context) error
	ProvisionTenant(ctx context.Context, tenant *model.Tenant) error
	ProvisionDevice(ctx context.Context, tenantID string, device *model.Device) error
	GetDevice(ctx context.Context, tenantID, deviceID string) (*model.Device, error)
	DeleteDevice(ctx context.Context, tenantID, deviceID string) error
	UpdateDeviceStatus(ctx context.Context, tenantID, deviceID, status string) error
	PrepareUserSession(ctx context.Context, sess *model.Session) error
	FreeUserSession(ctx context.Context, sessionID string) error
	RemoteTerminalAllowed(ctx context.Context, tenantID, deviceID string, groups []string) (bool, error)
}

// app is an app object
type app struct {
	store     store.DataStore
	inventory inventory.Client
	workflows workflows.Client
	Config
}

type Config struct {
	HaveAuditLogs bool
}

// NewApp initialize a new deviceconnect App
func New(ds store.DataStore, inv inventory.Client, wf workflows.Client, config ...Config) App {
	conf := Config{}
	for _, cfgIn := range config {
		if cfgIn.HaveAuditLogs {
			conf.HaveAuditLogs = true
		}
	}
	return &app{
		store:     ds,
		inventory: inv,
		workflows: wf,
		Config:    conf,
	}
}

// HealthCheck performs a health check and returns an error if it fails
func (a *app) HealthCheck(ctx context.Context) error {
	return a.store.Ping(ctx)
}

// ProvisionTenant provisions a new tenant
func (a *app) ProvisionTenant(ctx context.Context, tenant *model.Tenant) error {
	return a.store.ProvisionTenant(ctx, tenant.TenantID)
}

// ProvisionDevice provisions a new tenant
func (a *app) ProvisionDevice(
	ctx context.Context,
	tenantID string,
	device *model.Device,
) error {
	return a.store.ProvisionDevice(ctx, tenantID, device.ID)
}

// GetDevice returns a device
func (a *app) GetDevice(
	ctx context.Context,
	tenantID string,
	deviceID string,
) (*model.Device, error) {
	device, err := a.store.GetDevice(ctx, tenantID, deviceID)
	if err != nil {
		return nil, err
	} else if device == nil {
		return nil, ErrDeviceNotFound
	}
	return device, nil
}

// DeleteDevice provisions a new tenant
func (a *app) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	return a.store.DeleteDevice(ctx, tenantID, deviceID)
}

// UpdateDeviceStatus provisions a new tenant
func (a *app) UpdateDeviceStatus(
	ctx context.Context,
	tenantID, deviceID, status string,
) error {
	return a.store.UpsertDeviceStatus(ctx, tenantID, deviceID, status)
}

// PrepareUserSession prepares a new user session
func (a *app) PrepareUserSession(
	ctx context.Context,
	sess *model.Session,
) error {
	if sess == nil {
		return errors.New("nil Session")
	}
	if sess.ID == "" {
		sessID, err := uuid.NewRandom()
		if err != nil {
			return errors.Wrap(err, "failed to generate session ID")
		}
		sess.ID = sessID.String()
	}
	if err := sess.Validate(); err != nil {
		return errors.Wrap(err, "app: cannot create invalid Session")
	}

	device, err := a.store.GetDevice(ctx, sess.TenantID, sess.DeviceID)
	if err != nil {
		return err
	} else if device == nil {
		return ErrDeviceNotFound
	} else if device.Status != model.DeviceStatusConnected {
		return ErrDeviceNotConnected
	}

	err = a.store.AllocateSession(ctx, sess)
	if err != nil {
		return err
	}

	if a.HaveAuditLogs {
		err = a.workflows.SubmitAuditLog(ctx, workflows.AuditLog{
			Action: workflows.ActionCreate,
			Actor: workflows.Actor{
				ID:   sess.UserID,
				Type: workflows.ActorUser,
			},
			Object: workflows.Object{
				ID:   sess.ID,
				Type: workflows.ObjectTerminal,
				Terminal: &workflows.Terminal{
					DeviceID: sess.DeviceID,
				},
			},
			Change:  "User requested a new terminal session",
			EventTS: time.Now(),
		})
		if err != nil {
			err = errors.Wrap(err,
				"failed to submit audit log for creating terminal session",
			)
			_, e := a.store.DeleteSession(ctx, sess.ID)
			if e != nil {
				err = errors.Errorf(
					"%s: failed to clean up session state: %s",
					err.Error(), e.Error(),
				)
			}
			return err
		}
	}

	return nil
}

// FreeUserSession releases the session
func (a *app) FreeUserSession(
	ctx context.Context,
	sessionID string,
) error {
	sess, err := a.store.DeleteSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if a.HaveAuditLogs {
		err = a.workflows.SubmitAuditLog(ctx, workflows.AuditLog{
			Action: workflows.ActionDelete,
			Actor: workflows.Actor{
				ID:   sess.UserID,
				Type: workflows.ActorUser,
			},
			Object: workflows.Object{
				ID:   sess.ID,
				Type: workflows.ObjectTerminal,
				Terminal: &workflows.Terminal{
					DeviceID: sess.ID,
				},
			},
		})
	}
	return err
}

func buildRBACFilter(deviceID string, groups []string) model.SearchParams {
	searchParams := model.SearchParams{
		Page:    1,
		PerPage: 1,
		Filters: []model.FilterPredicate{
			{
				Scope:     model.InventoryGroupScope,
				Attribute: model.InventoryGroupAttributeName,
				Type:      "$in",
				Value:     groups,
			},
		},
		DeviceIDs: []string{deviceID},
	}
	return searchParams
}

func (a *app) RemoteTerminalAllowed(
	ctx context.Context,
	tenantID string,
	deviceID string,
	groups []string) (bool, error) {
	_, num, err := a.inventory.Search(ctx, tenantID, buildRBACFilter(deviceID, groups))
	if err != nil {
		return false, err
	} else if num != 1 {
		return false, nil
	}
	return true, nil
}
