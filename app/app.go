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

	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/deviceconnect/store"
)

// App interface describes app objects
type App interface {
	HealthCheck(context.Context) error
	ProvisionTenant(context.Context, *model.Tenant) error
}

// DeviceConnectApp is an app object
type DeviceConnectApp struct {
	store store.DataStore
}

// NewDeviceConnectApp returns a new DeviceConnectApp
func NewDeviceConnectApp(store store.DataStore) App {
	return &DeviceConnectApp{store: store}
}

// HealthCheck performs a health check and returns an error if it fails
func (a *DeviceConnectApp) HealthCheck(ctx context.Context) error {
	return nil
}

// ProvisionTenant provisions a new tenant
func (a *DeviceConnectApp) ProvisionTenant(ctx context.Context, tenant *model.Tenant) error {
	return a.store.ProvisionTenant(ctx, tenant.TenantID)
}
