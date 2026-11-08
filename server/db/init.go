package db

import (
	"context"
	"errors"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"schej.it/server/logger"
)

var Client *mongo.Client
var Db *mongo.Database
var EventsCollection *mongo.Collection
var UsersCollection *mongo.Collection
var DailyUserLogCollection *mongo.Collection
var FriendRequestsCollection *mongo.Collection
var EventResponsesCollection *mongo.Collection
var AttendeesCollection *mongo.Collection
var FoldersCollection *mongo.Collection
var FolderEventsCollection *mongo.Collection
var OtpCodesCollection *mongo.Collection

func Init() func() {
	cfg, err := loadTimeoutConfig()
	if err != nil {
		logger.StdErr.Fatalf("MongoDB config: %v", err)
	}
	HealthTimeout = cfg.Health

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost"
	}

	// Setters come before ApplyURI so that timeouts given in the URI take precedence
	clientOptions := options.Client().
		SetServerSelectionTimeout(cfg.ServerSelection).
		SetConnectTimeout(cfg.Connect).
		SetSocketTimeout(cfg.Socket).
		ApplyURI(mongoURI)

	// Establish mongodb connection (lazy, does not contact the server)
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()
	Client, err = mongo.Connect(connectCtx, clientOptions)
	if err != nil {
		// Invalid URI or options, waiting will not fix this. Don't log the URI, it may contain credentials
		logger.StdErr.Fatalf("MongoDB client could not be created: %v", err)
	}

	// Define mongodb database + collections
	Db = Client.Database("schej-it")
	EventsCollection = Db.Collection("events")
	UsersCollection = Db.Collection("users")
	DailyUserLogCollection = Db.Collection("dailyuserlogs")
	FriendRequestsCollection = Db.Collection("friendrequests")
	EventResponsesCollection = Db.Collection("eventResponses")
	AttendeesCollection = Db.Collection("attendees")
	FoldersCollection = Db.Collection("folders")
	FolderEventsCollection = Db.Collection("folderEvents")
	OtpCodesCollection = Db.Collection("otpCodes")

	// Make sure mongo is actually reachable
	if err := waitForMongo(cfg.Startup); err != nil {
		if isAuthError(err) {
			// Authentication failures require corrected credentials.
			logger.StdErr.Fatalf("MongoDB ping failed: %v", err)
		}
		// Keep running: the driver reconnects on its own once Mongo is back, and in the meantime the
		// frontend is still served, /api/health returns 503 and API requests return 503
		logger.StdErr.Printf("MongoDB unreachable after %v, starting in degraded mode (/api/health will return 503 until it is reachable): %v", cfg.Startup, err)
		go func() {
			for {
				time.Sleep(10 * time.Second)
				ctx, cancel := context.WithTimeout(context.Background(), HealthTimeout)
				err := Ping(ctx)
				cancel()
				if err == nil {
					logger.StdOut.Println("MongoDB is reachable again")
					createIndexes()
					return
				}
			}
		}()
	} else {
		logger.StdOut.Printf(
			"Connected to MongoDB (serverSelectionTimeout=%v connectTimeout=%v socketTimeout=%v healthTimeout=%v, URI parameters take precedence)",
			cfg.ServerSelection, cfg.Connect, cfg.Socket, cfg.Health,
		)
		createIndexes()
	}

	// Return a function to close the connection
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Client.Disconnect(ctx)
	}
}

// Ping checks that the primary is reachable. Returns an error if Init has not been called yet
func Ping(ctx context.Context) error {
	if Client == nil {
		return errors.New("mongo client is not initialized")
	}
	return Client.Ping(ctx, readpref.Primary())
}

// Pings mongo until it responds or `timeout` has passed. Authentication failures return
// immediately; other failures return the last error after the timeout.
func waitForMongo(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		err := Ping(ctx)
		if err == nil {
			return nil
		}
		if isAuthError(err) {
			return err
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

func createIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create TTL index so expired OTP docs are auto-deleted
	otpIndexModel := mongo.IndexModel{
		Keys:    bson.M{"expiresAt": 1},
		Options: options.Index().SetExpireAfterSeconds(0),
	}
	if _, err := OtpCodesCollection.Indexes().CreateOne(ctx, otpIndexModel); err != nil {
		logger.StdErr.Printf("Failed to create OTP TTL index: %v", err)
	}
}

// MongoDB backup / restore commands

// Backup
// mongodump --uri="mongodb://localhost:27017" --db=schej-it

// Restore
// mongorestore --uri="mongodb://localhost:27017" --drop --db=schej-it ./dump
