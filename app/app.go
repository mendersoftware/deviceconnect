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

import "context"

// App interface describes app objects
type App interface {
	HealthCheck(context.Context) error
}

// DeviceConnectApp is an app object
type DeviceConnectApp struct{}

// NewDeviceConnectApp returns a new DeviceConnectApp
func NewDeviceConnectApp() App {
	return &DeviceConnectApp{}
}

// HealthCheck performs a health check and returns an error if it fails
func (a *DeviceConnectApp) HealthCheck(context.Context) error {
	return nil
}
