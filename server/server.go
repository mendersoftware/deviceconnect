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

package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/mendersoftware/go-lib-micro/log"
	"golang.org/x/sys/unix"

	api "github.com/mendersoftware/deviceconnect/api/http"
	"github.com/mendersoftware/deviceconnect/app"
	clientnats "github.com/mendersoftware/deviceconnect/client/nats"
	dconfig "github.com/mendersoftware/deviceconnect/config"
	"github.com/mendersoftware/deviceconnect/store"
)

// InitAndRun initializes the server and runs it
func InitAndRun(conf config.Reader, dataStore store.DataStore) error {
	ctx := context.Background()

	log.Setup(conf.GetBool(dconfig.SettingDebugLog))
	l := log.FromContext(ctx)

	client := &clientnats.Client{}
	if natsURI := config.Config.GetString(dconfig.SettingNatsURI); natsURI != "" {
		if err := client.Connect(ctx, natsURI); err != nil {
			return err
		}
	}
	deviceConnectApp := app.NewDeviceConnectApp(dataStore, client)

	router, err := api.NewRouter(deviceConnectApp)
	if err != nil {
		l.Fatal(err)
	}

	var listen = conf.GetString(dconfig.SettingListen)
	srv := &http.Server{
		Addr:    listen,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			l.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, unix.SIGINT, unix.SIGTERM)
	<-quit

	l.Info("server shutdown")

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctxWithTimeout); err != nil {
		l.Fatal("error when shutting down the server ", err)
	}

	l.Info("server exiting")
	return nil
}
