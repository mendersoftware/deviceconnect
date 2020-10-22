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

package useradm

import (
	"context"
	"net/http"

	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/pkg/errors"
)

// useradm errors
var (
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrTooManyRequests = errors.New("too many requests")
	ErrInternalError   = errors.New("internal error")
)

const (
	verifyURI                = "/api/internal/v1/useradm/auth/verify"
	headerAuthorization      = "Authorization"
	headerForwardedForURI    = "X-Forwarded-URI"
	headerForwardedForMethod = "X-Forwarded-Method"
)

// HTTPClient interfacee
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// ClientInterface is the interface of a useradm client
type ClientInterface interface {
	Verify(ctx context.Context, token string, method string, uri string) error
}

// Client provides a useradm client
type Client struct {
	URI        string
	httpClient HTTPClient
}

// NewClient returns a new Client
func NewClient(URI string) *Client {
	return &Client{
		URI:        URI,
		httpClient: &http.Client{},
	}
}

// Verify verifies a user JWT token
func (c *Client) Verify(ctx context.Context, token string, method string, uri string) error {
	l := log.FromContext(ctx)

	verifyURI := c.URI + verifyURI
	req, err := http.NewRequest(http.MethodPost, verifyURI, nil)
	if err != nil {
		l.Error(errors.Wrap(err, "error while creating the request"))
		return ErrInternalError
	}
	req.Header.Add(headerAuthorization, "Bearer "+token)
	req.Header.Add(headerForwardedForURI, uri)
	req.Header.Add(headerForwardedForMethod, method)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		l.Error(errors.Wrap(err, "error while making the http request"))
		return ErrInternalError
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	} else if resp.StatusCode == http.StatusForbidden {
		return ErrForbidden
	} else if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	} else if resp.StatusCode == http.StatusTooManyRequests {
		return ErrTooManyRequests
	} else {
		return ErrInternalError
	}
}

// GetHTTPStatusCodeFromError returns the HTTP status code from an error
func GetHTTPStatusCodeFromError(err error) int {
	var code int
	switch err {
	case ErrForbidden:
		code = http.StatusForbidden
	case ErrUnauthorized:
		code = http.StatusUnauthorized
	case ErrTooManyRequests:
		code = http.StatusTooManyRequests
	default:
		code = http.StatusInternalServerError
	}
	return code
}
