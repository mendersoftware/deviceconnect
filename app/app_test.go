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

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/mock"

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

	app := NewDeviceConnectApp(store, nil)

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

	app := NewDeviceConnectApp(store, nil)

	ctx := context.Background()
	res := app.ProvisionTenant(ctx, &model.Tenant{TenantID: tenantID})
	assert.Equal(t, err, res)

	store.AssertExpectations(t)
}
