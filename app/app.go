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

	clientnats "github.com/mendersoftware/deviceconnect/client/nats"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/deviceconnect/store"
)

// App interface describes app objects
type App interface {
	HealthCheck(ctx context.Context) error
	ProvisionTenant(ctx context.Context, tenant *model.Tenant) error
	ProvisionDevice(ctx context.Context, tenantID string, device *model.Device) error
	DeleteDevice(ctx context.Context, tenantID string, deviceID string) error
	UpdateDeviceStatus(ctx context.Context, tenantID string, deviceID string, status string) error
}

// DeviceConnectApp is an app object
type DeviceConnectApp struct {
	store  store.DataStore
	client clientnats.ClientInterface
}

// NewDeviceConnectApp returns a new DeviceConnectApp
func NewDeviceConnectApp(store store.DataStore, client clientnats.ClientInterface) App {
	return &DeviceConnectApp{store: store, client: client}
}

// HealthCheck performs a health check and returns an error if it fails
func (a *DeviceConnectApp) HealthCheck(ctx context.Context) error {
	return a.store.Ping(ctx)
}

// ProvisionTenant provisions a new tenant
func (a *DeviceConnectApp) ProvisionTenant(ctx context.Context, tenant *model.Tenant) error {
	return a.store.ProvisionTenant(ctx, tenant.TenantID)
}

// ProvisionDevice provisions a new tenant
func (a *DeviceConnectApp) ProvisionDevice(ctx context.Context, tenantID string, device *model.Device) error {
	return a.store.ProvisionDevice(ctx, tenantID, device.DeviceID)
}

// DeleteDevice provisions a new tenant
func (a *DeviceConnectApp) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	return a.store.DeleteDevice(ctx, tenantID, deviceID)
}

// UpdateDeviceStatus provisions a new tenant
func (a *DeviceConnectApp) UpdateDeviceStatus(ctx context.Context, tenantID, deviceID, status string) error {
	return a.store.UpdateDeviceStatus(ctx, tenantID, deviceID, status)
}
