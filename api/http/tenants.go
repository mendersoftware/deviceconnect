// Copyright 2019 Northern.tech AS
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
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mendersoftware/deviceconnect/app"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/pkg/errors"
)

// TenantsController contains status-related end-points
type TenantsController struct {
	app app.App
}

// NewTenantsController returns a new TenantsController
func NewTenantsController(app app.App) *TenantsController {
	return &TenantsController{app: app}
}

// Provision responds to POST /tenants
func (h TenantsController) Provision(c *gin.Context) {
	rawData, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "bad request",
		})
		return
	}

	tenant := &model.Tenant{}
	if err = json.Unmarshal(rawData, tenant); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.Wrap(err, "invalid payload").Error(),
		})
		return
	} else if tenant.TenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "tenant_id is empty",
		})
		return
	}

	ctx := c.Request.Context()
	if err = h.app.ProvisionTenant(ctx, tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errors.Wrap(err, "error provisioning the tenant").Error(),
		})
		return
	}

	c.Writer.WriteHeader(http.StatusCreated)
}
