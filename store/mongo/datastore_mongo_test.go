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

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/stretchr/testify/assert"
)

func TestPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestPing in short mode.")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	ds := NewDataStoreWithClient(db.Client(), config.Config)
	err := ds.Ping(ctx)
	assert.NoError(t, err)
}

func TestProvisionTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestPing in short mode.")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	ds := NewDataStoreWithClient(db.Client(), config.Config)
	err := ds.ProvisionTenant(ctx, "1234")
	assert.NoError(t, err)
}

func TestProvisionAndDeleteDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestPing in short mode.")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	const (
		tenantID = "1234"
		deviceID = "abcd"
	)

	ds := NewDataStoreWithClient(db.Client(), config.Config)
	err := ds.ProvisionDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)

	device, err := ds.GetDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.Equal(t, deviceID, device.ID)

	err = ds.DeleteDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)

	device, err = ds.GetDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.Nil(t, device)
}

func TestUpsertDeviceStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestPing in short mode.")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	const (
		tenantID = "1234"
		deviceID = "abcd"
	)

	ds := NewDataStoreWithClient(db.Client(), config.Config)
	err := ds.ProvisionDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)

	device, err := ds.GetDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.Equal(t, model.DeviceStatusDisconnected, device.Status)

	err = ds.UpsertDeviceStatus(ctx, tenantID, deviceID, model.DeviceStatusConnected)
	assert.NoError(t, err)

	device, err = ds.GetDevice(ctx, tenantID, deviceID)
	assert.NoError(t, err)
	assert.Equal(t, model.DeviceStatusConnected, device.Status)

	const anotherDeviceID = "efgh"
	err = ds.UpsertDeviceStatus(ctx, tenantID, anotherDeviceID, model.DeviceStatusConnected)
	assert.NoError(t, err)

	device, err = ds.GetDevice(ctx, tenantID, anotherDeviceID)
	assert.NoError(t, err)
	assert.Equal(t, model.DeviceStatusConnected, device.Status)

	err = ds.UpsertDeviceStatus(ctx, tenantID, anotherDeviceID, model.DeviceStatusDisconnected)
	assert.NoError(t, err)

	device, err = ds.GetDevice(ctx, tenantID, anotherDeviceID)
	assert.NoError(t, err)
	assert.Equal(t, model.DeviceStatusDisconnected, device.Status)
}

func TestUpsertSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestPing in short mode.")
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	const (
		tenantID = "1234"
		userID   = "abcd"
		deviceID = "efgh"
	)

	ds := NewDataStoreWithClient(db.Client(), config.Config)
	session, err := ds.UpsertSession(ctx, tenantID, userID, deviceID)
	assert.NoError(t, err)
	assert.Equal(t, userID, session.UserID)
	assert.Equal(t, deviceID, session.DeviceID)
	assert.Equal(t, model.SessionStatusDisconnected, session.Status)
	time.Sleep(time.Minute * 10)

	//err = ds.UpdateSessionStatus(ctx, tenantID, session.ID, model.DeviceStatusConnected)
	//assert.NoError(t, err)

	//session, err = ds.GetSession(ctx, tenantID, session.ID)
	//assert.NoError(t, err)
	//assert.NotNil(t, session)
	//assert.Equal(t, model.SessiontatusConnected, session.Status)
}
