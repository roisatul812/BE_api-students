package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// MongoConfig menyimpan konfigurasi koneksi MongoDB.
type MongoConfig struct {
	URI            string
	DatabaseName   string
	ConnectTimeout time.Duration
	PingTimeout    time.Duration
	MaxPoolSize    uint64
}

// LoadMongoConfig memuat konfigurasi MongoDB dari environment variable dengan nilai fallback yang aman.
func LoadMongoConfig() MongoConfig {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	dbName := os.Getenv("MONGO_DATABASE")
	if dbName == "" {
		dbName = "praktikum_backend"
	}

	maxPool := uint64(20)
	if poolStr := os.Getenv("MONGO_MAX_POOL"); poolStr != "" {
		if val, err := strconv.ParseUint(poolStr, 10, 64); err == nil && val > 0 {
			maxPool = val
		}
	}

	return MongoConfig{
		URI:            uri,
		DatabaseName:   dbName,
		ConnectTimeout: 5 * time.Second,
		PingTimeout:    5 * time.Second,
		MaxPoolSize:    maxPool,
	}
}

// ConnectMongo menginisialisasi client MongoDB, melakukan ping dengan timeout,
// dan mengembalikan client yang siap digunakan.
// Berbeda dari pgxpool, mongo.Connect pada driver v2 tidak langsung menghubungi server.
// Karena itu, client.Ping eksplisit dengan context timeout wajib dilakukan agar kegagalan koneksi
// langsung terdeteksi saat inisialisasi aplikasi.
func ConnectMongo(ctx context.Context, cfg MongoConfig) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(cfg.URI).
		SetServerSelectionTimeout(cfg.ConnectTimeout).
		SetMaxPoolSize(cfg.MaxPoolSize)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan client MongoDB: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		discCtx, discCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer discCancel()
		_ = client.Disconnect(discCtx)
		return nil, fmt.Errorf("ping MongoDB ke %s gagal: %w", cfg.URI, err)
	}

	return client, nil
}

// NewMongoClient menyiapkan client MongoDB menggunakan konfigurasi environment default.
func NewMongoClient(ctx context.Context) (*mongo.Client, error) {
	cfg := LoadMongoConfig()
	return ConnectMongo(ctx, cfg)
}

// DisconnectMongo menutup seluruh koneksi MongoDB client secara anggun dengan context timeout.
func DisconnectMongo(client *mongo.Client, timeout time.Duration) error {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.Disconnect(ctx)
}

// GetMongoDatabase mengembalikan handle database MongoDB dari client.
func GetMongoDatabase(client *mongo.Client, databaseName string) *mongo.Database {
	if client == nil {
		return nil
	}
	if databaseName == "" {
		databaseName = "praktikum_backend"
	}
	return client.Database(databaseName)
}
