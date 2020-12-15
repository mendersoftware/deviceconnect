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

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"

	dconfig "github.com/mendersoftware/deviceconnect/config"
	"github.com/mendersoftware/deviceconnect/model"
	"github.com/mendersoftware/deviceconnect/store"
)

const (
	// DevicesCollectionName refers to the name of the collection of stored devices
	DevicesCollectionName = "devices"

	// SessionsCollectionName refers to the name of the collection of sessions
	SessionsCollectionName = "sessions"

	dbFieldStatus    = "status"
	dbFieldCreatedTs = "created_ts"
	dbFieldUpdatedTs = "updated_ts"
)

// SetupDataStore returns the mongo data store and optionally runs migrations
func SetupDataStore(automigrate bool) (store.DataStore, error) {
	ctx := context.Background()
	dbClient, err := NewClient(ctx, config.Config)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to connect to db: %v", err))
	}
	err = doMigrations(ctx, dbClient, automigrate)
	if err != nil {
		return nil, err
	}
	dataStore := NewDataStoreWithClient(dbClient)
	return dataStore, nil
}

func doMigrations(ctx context.Context, client *mongo.Client,
	automigrate bool) error {
	db := config.Config.GetString(dconfig.SettingDbName)
	dbs, err := migrate.GetTenantDbs(ctx, client, mstore.IsTenantDb(db))
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
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
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
}

// NewDataStoreWithClient initializes a DataStore object
func NewDataStoreWithClient(client *mongo.Client) store.DataStore {
	return &DataStoreMongo{
		client: client,
	}
}

// Ping verifies the connection to the database
func (db *DataStoreMongo) Ping(ctx context.Context) error {
	res := db.client.Database(DbName).RunCommand(ctx, bson.M{"ping": 1})
	return res.Err()
}

// ProvisionTenant provisions a new tenant
func (db *DataStoreMongo) ProvisionTenant(ctx context.Context, tenantID string) error {
	dbname := mstore.DbNameForTenant(tenantID, DbName)
	return Migrate(ctx, dbname, DbVersion, db.client, true)
}

// ProvisionDevice provisions a new device
func (db *DataStoreMongo) ProvisionDevice(ctx context.Context, tenantID, deviceID string) error {
	dbname := mstore.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	now := time.Now().UTC()

	updateOpts := &mopts.UpdateOptions{}
	updateOpts.SetUpsert(true)
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": deviceID},
		bson.M{
			"$setOnInsert": bson.M{
				dbFieldStatus:    model.DeviceStatusUnknown,
				dbFieldCreatedTs: &now,
			},
		},
		updateOpts,
	)
	return err
}

// DeleteDevice deletes a device
func (db *DataStoreMongo) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	dbname := mstore.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	_, err := coll.DeleteOne(ctx, bson.M{"_id": deviceID})
	return err
}

// GetDevice returns a device
func (db *DataStoreMongo) GetDevice(
	ctx context.Context,
	tenantID string,
	deviceID string,
) (*model.Device, error) {
	dbname := mstore.DbNameForTenant(tenantID, DbName)
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

// UpsertDeviceStatus upserts the connection status of a device
func (db *DataStoreMongo) UpsertDeviceStatus(
	ctx context.Context,
	tenantID string,
	deviceID string,
	status string,
) error {
	dbname := mstore.DbNameForTenant(tenantID, DbName)
	coll := db.client.Database(dbname).Collection(DevicesCollectionName)

	updateOpts := &mopts.UpdateOptions{}
	updateOpts.SetUpsert(true)

	now := time.Now().UTC()

	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": deviceID},
		bson.M{
			"$set": bson.M{
				dbFieldStatus:    status,
				dbFieldUpdatedTs: &now,
			},
			"$setOnInsert": bson.M{
				dbFieldCreatedTs: &now,
			},
		},
		updateOpts,
	)

	return err
}

// AllocateSession allocates a new session.
func (db *DataStoreMongo) AllocateSession(ctx context.Context, sess *model.Session) error {

	if err := sess.Validate(); err != nil {
		return errors.Wrap(err, "store: cannot allocate invalid Session")
	}

	dbname := mstore.DbNameForTenant(sess.TenantID, DbName)
	coll := db.client.Database(dbname).Collection(SessionsCollectionName)

	_, err := coll.InsertOne(ctx, sess)
	if err != nil {
		return errors.Wrap(err, "store: failed to allocate session")
	}

	return nil
}

// DeleteSession deletes a session
func (db *DataStoreMongo) DeleteSession(
	ctx context.Context, sessionID string,
) (*model.Session, error) {
	dbname := mstore.DbFromContext(ctx, DbName)
	collSess := db.client.Database(dbname).
		Collection(SessionsCollectionName)

	sess := new(model.Session)
	err := collSess.FindOneAndDelete(
		ctx, bson.D{{Key: "_id", Value: sessionID}},
	).Decode(sess)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, store.ErrSessionNotFound
		} else {
			return nil, err
		}
	}
	if idty := identity.FromContext(ctx); idty != nil {
		sess.TenantID = idty.Tenant
	}
	return sess, nil
}

// GetSession returns a device
func (db *DataStoreMongo) GetSession(
	ctx context.Context,
	sessionID string,
) (*model.Session, error) {
	collSess := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(SessionsCollectionName)

	session := &model.Session{}
	err := collSess.
		FindOne(ctx, bson.M{"_id": sessionID}).
		Decode(session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, store.ErrSessionNotFound
		}
		return nil, err
	}
	idty := identity.FromContext(ctx)
	if idty != nil {
		session.TenantID = idty.Tenant
	}

	return session, nil
}

// Close disconnects the client
func (db *DataStoreMongo) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := db.client.Disconnect(ctx)
	return err
}

//nolint:unused
func (db *DataStoreMongo) DropDatabase() error {
	ctx := context.Background()
	err := db.client.Database(DbName).Drop(ctx)
	return err
}
