package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"api-students/database"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// TestLoadMongoConfig_Default menguji konfigurasi bawaan saat environment variable tidak diset.
func TestLoadMongoConfig_Default(t *testing.T) {
	origURI := os.Getenv("MONGO_URI")
	origDB := os.Getenv("MONGO_DATABASE")
	origPool := os.Getenv("MONGO_MAX_POOL")
	defer func() {
		os.Setenv("MONGO_URI", origURI)
		os.Setenv("MONGO_DATABASE", origDB)
		os.Setenv("MONGO_MAX_POOL", origPool)
	}()

	os.Unsetenv("MONGO_URI")
	os.Unsetenv("MONGO_DATABASE")
	os.Unsetenv("MONGO_MAX_POOL")

	cfg := database.LoadMongoConfig()
	if cfg.URI != "mongodb://localhost:27017" {
		t.Errorf("expected default URI mongodb://localhost:27017, got %s", cfg.URI)
	}
	if cfg.DatabaseName != "praktikum_backend" {
		t.Errorf("expected default DB praktikum_backend, got %s", cfg.DatabaseName)
	}
	if cfg.MaxPoolSize != 20 {
		t.Errorf("expected default MaxPoolSize 20, got %d", cfg.MaxPoolSize)
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("expected ConnectTimeout 5s, got %v", cfg.ConnectTimeout)
	}
}

// TestLoadMongoConfig_CustomEnv menguji parsing konfigurasi kustom dari environment variable.
func TestLoadMongoConfig_CustomEnv(t *testing.T) {
	origURI := os.Getenv("MONGO_URI")
	origDB := os.Getenv("MONGO_DATABASE")
	origPool := os.Getenv("MONGO_MAX_POOL")
	defer func() {
		os.Setenv("MONGO_URI", origURI)
		os.Setenv("MONGO_DATABASE", origDB)
		os.Setenv("MONGO_MAX_POOL", origPool)
	}()

	os.Setenv("MONGO_URI", "mongodb://custom-host:27018")
	os.Setenv("MONGO_DATABASE", "custom_db")
	os.Setenv("MONGO_MAX_POOL", "50")

	cfg := database.LoadMongoConfig()
	if cfg.URI != "mongodb://custom-host:27018" {
		t.Errorf("expected custom URI, got %s", cfg.URI)
	}
	if cfg.DatabaseName != "custom_db" {
		t.Errorf("expected custom DB, got %s", cfg.DatabaseName)
	}
	if cfg.MaxPoolSize != 50 {
		t.Errorf("expected custom MaxPoolSize 50, got %d", cfg.MaxPoolSize)
	}
}

// TestConnectMongo_UnreachableServer menguji penanganan error saat server MongoDB tidak dapat dihubungi.
func TestConnectMongo_UnreachableServer(t *testing.T) {
	ctx := context.Background()
	cfg := database.MongoConfig{
		URI:            "mongodb://127.0.0.1:59999", // Port yang tidak aktif
		DatabaseName:   "test_unreachable",
		ConnectTimeout: 500 * time.Millisecond,
		PingTimeout:    500 * time.Millisecond,
		MaxPoolSize:    5,
	}

	client, err := database.ConnectMongo(ctx, cfg)
	if err == nil {
		if client != nil {
			_ = database.DisconnectMongo(client, 1*time.Second)
		}
		t.Fatalf("koneksi ke port 59999 seharusnya gagal, tetapi berhasil")
	}

	t.Logf("Error penanganan kegagalan server berhasil ditangkap: %v", err)
}

// TestDisconnectMongo_NilClient menguji bahwa disconnect pada nil client tidak menyebabkan kepanikan (panic).
func TestDisconnectMongo_NilClient(t *testing.T) {
	err := database.DisconnectMongo(nil, 1*time.Second)
	if err != nil {
		t.Errorf("expected nil error on nil client disconnect, got %v", err)
	}
}

// TestGetMongoDatabase_NilClient menguji bahwa perolehan database dari nil client mengembalikan nil.
func TestGetMongoDatabase_NilClient(t *testing.T) {
	db := database.GetMongoDatabase(nil, "any_db")
	if db != nil {
		t.Errorf("expected nil database on nil client, got %v", db)
	}
}

// TestMongo_Integration_ConnectAndPing adalah INTEGRATION TEST NYATA terhadap MongoDB lokal.
// Memverifikasi konektivitas, ping, database handle, dan disconnect yang anggun.
func TestMongo_Integration_ConnectAndPing(t *testing.T) {
	_ = godotenv.Load("../.env")

	cfg := database.LoadMongoConfig()
	ctx := context.Background()

	client, err := database.ConnectMongo(ctx, cfg)
	if err != nil {
		t.Skipf("MongoDB lokal tidak tersedia di %s: %v", cfg.URI, err)
		return
	}

	// 1. Verifikasi ping aktif
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		t.Fatalf("ping ke MongoDB gagal: %v", err)
	}
	t.Logf("Koneksi MongoDB berhasil dan ping responsif ke: %s", cfg.URI)

	// 2. Verifikasi perolehan database handle
	db := database.GetMongoDatabase(client, cfg.DatabaseName)
	if db == nil {
		t.Fatalf("GetMongoDatabase mengembalikan nil")
	}
	if db.Name() != cfg.DatabaseName {
		t.Errorf("nama database diharapkan %s, didapat %s", cfg.DatabaseName, db.Name())
	}

	// 3. Verifikasi penutupan koneksi (disconnect)
	if err := database.DisconnectMongo(client, 3*time.Second); err != nil {
		t.Errorf("disconnect MongoDB gagal: %v", err)
	}

	// 4. Verifikasi setelah disconnect operasi ping ditolak
	afterCtx, afterCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer afterCancel()
	if err := client.Ping(afterCtx, readpref.Primary()); err == nil {
		t.Errorf("ping setelah disconnect seharusnya gagal, tapi berhasil")
	} else {
		t.Logf("Verifikasi disconnect sukses: operasi setelah disconnect ditolak (%v)", err)
	}
}

// TestDatabase_DualCoexistence_PostgresAndMongo menguji koeksistensi kedua database (PostgreSQL & MongoDB)
// berjalan bersamaan tanpa saling mengganggu.
func TestDatabase_DualCoexistence_PostgresAndMongo(t *testing.T) {
	_ = godotenv.Load("../.env")
	ctx := context.Background()

	// Inisialisasi PostgreSQL
	pgPool, err := database.Connect(ctx)
	if err != nil {
		t.Skipf("PostgreSQL tidak tersedia: %v", err)
		return
	}
	defer pgPool.Close()

	// Inisialisasi MongoDB
	mongoCfg := database.LoadMongoConfig()
	mongoClient, err := database.ConnectMongo(ctx, mongoCfg)
	if err != nil {
		t.Skipf("MongoDB tidak tersedia: %v", err)
		return
	}
	defer func() {
		_ = database.DisconnectMongo(mongoClient, 2*time.Second)
	}()

	// Ping kedua database
	hCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := pgPool.Ping(hCtx); err != nil {
		t.Fatalf("ping PostgreSQL gagal: %v", err)
	}
	if err := mongoClient.Ping(hCtx, readpref.Primary()); err != nil {
		t.Fatalf("ping MongoDB gagal: %v", err)
	}

	t.Logf("PostgreSQL dan MongoDB berhasil terhubung dan responsif secara berdampingan!")
}
