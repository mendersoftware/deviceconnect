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

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	"github.com/stretchr/testify/assert"
)

func TestMigration_1_0_0(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMigration_1_0_0 in short mode.")
	}
	ctx := context.Background()

	testCases := map[string]struct {
		// ST or MT naming convention
		db    string
		dbVer string
		err   error
	}{
		"ST, no index, 0.0.0": {
			db:    DbName,
			dbVer: "",
		},
		"ST, no index, 0.0.1": {
			db:    DbName,
			dbVer: "0.0.1",
		},
		"ST, with index, 0.0.1": {
			db:    DbName,
			dbVer: "0.0.1",
		},
		"MT, no index, 0.0.0": {
			db:    DbName + "-59afdb71c704db002a86ad95",
			dbVer: "",
		},
		"MT, no index, 0.0.1": {
			db:    DbName + "-59afdb71c704db002a86ad95",
			dbVer: "0.0.1",
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db.Wipe()
		c := db.Client()

		// setup existing migrations
		if tc.dbVer != "" {
			ver, err := migrate.NewVersion(tc.dbVer)
			assert.NoError(t, err)
			_ = migrate.UpdateMigrationInfo(ctx, *ver, c, tc.db)
		}

		migrations := []migrate.Migration{
			&migration1_0_0{
				client: c,
				db:     tc.db,
			},
		}

		m := migrate.SimpleMigrator{
			Client:      c,
			Db:          tc.db,
			Automigrate: true,
		}

		err := m.Apply(ctx, migrate.MakeVersion(1, 0, 0), migrations)
		assert.NoError(t, err)
	}
}
