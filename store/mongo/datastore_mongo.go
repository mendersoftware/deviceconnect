// Copyright 2020 Northern.tech AS
//
//    All Rights Reserved

package mongo

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	"github.com/mendersoftware/go-lib-micro/store"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	dconfig "github.com/mendersoftware/deviceconnect/config"
	"github.com/mendersoftware/deviceconnect/model"
)

const (
	// DevicesCollectionName refers to the name of the collection of stored devices
	DevicesCollectionName = "devices"

	// SessionsCollectionName refers to the name of the collection of sessions
	SessionsCollectionName = "sessions"
)

// SetupDataStore returns the mongo data store and optionally runs migrations
func SetupDataStore(automigrate bool) (*DataStoreMongo, error) {
	ctx := context.Background()
	dbClient, err := NewClient(ctx, config.Config)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to connect to db: %v", err))
	}
	err = doMigrations(ctx, dbClient, automigrate)
	if err != nil {
		return nil, err
	}
	dataStore := NewDataStoreWithClient(dbClient, config.Config)
	return dataStore, nil
}

func doMigrations(ctx context.Context, client *mongo.Client,
	automigrate bool) error {
	db := config.Config.GetString(dconfig.SettingDbName)
	dbs, err := migrate.GetTenantDbs(ctx, client, store.IsTenantDb(db))
	if err != nil {
		return errors.Wrap(err, "failed go retrieve tenant DBs")
	}
	if len(dbs) == 0 {
		dbs = []string{DbName}
	}

	for _, d := range dbs {
		err := Migrate(ctx, d, DbVersion, client, automigrate)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to run migrations: %v", err))
		}
	}
	return nil
}

func disconnectClient(parentCtx context.Context, client *mongo.Client) {
	ctx, cancel := context.WithTimeout(parentCtx, 1*time.Second)
	defer cancel()
	client.Disconnect(ctx)
}

// NewClient returns a mongo client
func NewClient(ctx context.Context, c config.Reader) (*mongo.Client, error) {

	clientOptions := mopts.Client()
	mongoURL := c.GetString(dconfig.SettingMongo)
	if !strings.Contains(mongoURL, "://") {
		return nil, errors.Errorf("Invalid mongoURL %q: missing schema.",
			mongoURL)
	}
	clientOptions.ApplyURI(mongoURL)

	username := c.GetString(dconfig.SettingDbUsername)
	if username != "" {
		credentials := mopts.Credential{
			Username: c.GetString(dconfig.SettingDbUsername),
		}
		password := c.GetString(dconfig.SettingDbPassword)
		if password != "" {
			credentials.Password = password
			credentials.PasswordSet = true
		}
		clientOptions.SetAuth(credentials)
	}

	if c.GetBool(dconfig.SettingDbSSL) {
		tlsConfig := &tls.Config{}
		tlsConfig.InsecureSkipVerify = c.GetBool(dconfig.SettingDbSSLSkipVerify)
		clientOptions.SetTLSConfig(tlsConfig)
	}

	// Set writeconcern to acknowlage after write has propagated to the
	// mongod instance and commited to the file system journal.
	var wc *writeconcern.WriteConcern
	wc.WithOptions(writeconcern.W(1), writeconcern.J(true))
	clientOptions.SetWriteConcern(wc)

	// Set 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to mongo server")
	}

	// Validate connection
	if err = client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "Error reaching mongo server")
	}

	return client, nil
}

// DataStoreMongo is the data storage service
type DataStoreMongo struct {
	// client holds the reference to the client used to communicate with the
	// mongodb server.
	client *mongo.Client
	// dbName contains the name of the auditlogs database.
	dbName string
}

// NewDataStoreWithClient initializes a DataStore object
func NewDataStoreWithClient(client *mongo.Client, c config.Reader) *DataStoreMongo {
	dbName := c.GetString(dconfig.SettingDbName)

	return &DataStoreMongo{
		client: client,
		dbName: dbName,
	}
}

// Ping verifies the connection to the database
func (db *DataStoreMongo) Ping(ctx context.Context) error {
	res := db.client.Database(DbName).RunCommand(ctx, bson.M{"ping": 1})
	return res.Err()
}

// ProvisionTenant provisions a new tenant
func (db *DataStoreMongo) ProvisionTenant(ctx context.Context, tenantID string) error {
	dbname := store.DbNameForTenant(tenantID, DbName)
	return Migrate(ctx, dbname, DbVersion, db.client, true)
}

// ProvisionDevice provisions a new device
func (db *DataStoreMongo) ProvisionDevice(ctx context.Context, tenantID string, deviceID string) error {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	updateOpts := &mopts.UpdateOptions{}
	updateOpts.SetUpsert(true)
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": deviceID},
		bson.M{
			"$setOnInsert": bson.M{"status": model.DeviceStatusDisconnected},
		},
		updateOpts,
	)

	return err
}

// DeleteDevice deletes a device
func (db *DataStoreMongo) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	_, err := coll.DeleteOne(ctx, bson.M{"_id": deviceID})
	return err
}

// GetDevice returns a device
func (db *DataStoreMongo) GetDevice(ctx context.Context, tenantID, deviceID string) (*model.Device, error) {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	cur := coll.FindOne(ctx, bson.M{"_id": deviceID})

	device := &model.Device{}
	if err := cur.Decode(&device); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return device, nil
}

// UpdateDeviceStatus updates a device status
func (db *DataStoreMongo) UpdateDeviceStatus(ctx context.Context, tenantID string, deviceID string, status string) error {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	updateOpts := &mopts.UpdateOptions{}
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": deviceID},
		bson.M{
			"$set": bson.M{"status": status},
		},
		updateOpts,
	)

	return err
}

// UpsertSession upserts a user session connecting to a device
func (db *DataStoreMongo) UpsertSession(ctx context.Context, tenantID, userID, deviceID string) (*model.Session, error) {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(SessionsCollectionName)

	findOneAndUpdateOpts := &mopts.FindOneAndUpdateOptions{}
	findOneAndUpdateOpts.SetUpsert(true)
	findOneAndUpdateOpts.SetReturnDocument(mopts.After)
	res := coll.FindOneAndUpdate(ctx,
		bson.M{
			"user_id":   userID,
			"device_id": deviceID,
		},
		bson.M{
			"$setOnInsert": bson.M{
				"_id":       uuid.NewV4().String(),
				"user_id":   userID,
				"device_id": deviceID,
			},
			"$set": bson.M{
				"status": model.SessionStatusDisconnected,
			},
		},
		findOneAndUpdateOpts,
	)

	session := &model.Session{}
	if err := res.Decode(session); err != nil {
		return nil, err
	}

	return session, nil
}

// UpdateSessionStatus updates a session status
func (db *DataStoreMongo) UpdateSessionStatus(ctx context.Context, tenantID, sessionID, status string) error {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(SessionsCollectionName)

	updateOpts := &mopts.UpdateOptions{}
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set": bson.M{"status": status},
		},
		updateOpts,
	)

	return err
}

// GetSession returns a device
func (db *DataStoreMongo) GetSession(ctx context.Context, tenantID, sessionID string) (*model.Session, error) {
	dbname := store.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(SessionsCollectionName)

	cur := coll.FindOne(ctx, bson.M{"_id": sessionID})

	session := &model.Session{}
	if err := cur.Decode(session); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return session, nil
}

// Close disconnects the client
func (db *DataStoreMongo) Close() {
	ctx := context.Background()
	disconnectClient(ctx, db.client)
}

func (db *DataStoreMongo) dropDatabase() error {
	ctx := context.Background()
	err := db.client.Database(db.dbName).Drop(ctx)
	return err
}
