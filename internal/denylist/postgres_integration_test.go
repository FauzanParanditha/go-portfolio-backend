package denylist

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// resolveDSN mengambil DSN Postgres dari env DB_DSN. Bila belum ada di env,
// coba muat dari file .env di root repo (naik dua level dari paket ini).
func resolveDSN(t *testing.T) string {
	t.Helper()

	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		return dsn
	}

	// internal/denylist -> naik dua tingkat ke root repo tempat .env berada.
	if wd, err := os.Getwd(); err == nil {
		envPath := filepath.Join(wd, "..", "..", ".env")
		if vals, err := godotenv.Read(envPath); err == nil {
			if dsn := vals["DB_DSN"]; dsn != "" {
				return dsn
			}
		}
	}
	return ""
}

// openTestDB membuka koneksi ke Postgres uji. Bila DSN tak tersedia atau DB
// tak bisa dihubungi, test di-skip otomatis (bukan gagal) — sesuai konvensi
// integration test guarded di repo ini.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := resolveDSN(t)
	if dsn == "" {
		t.Skip("DB_DSN tidak tersedia; lewati integration test Postgres denylist")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("gagal open DB (%v); lewati integration test", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("gagal ambil sql.DB (%v); lewati integration test", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Postgres tak bisa dihubungi (%v); lewati integration test", err)
	}

	// Guard tambahan: bila tabel revoked_tokens belum di-migrate, skip agar
	// kegagalan lingkungan (migration belum jalan) tidak tersamar sebagai bug
	// logika pada test.
	if !db.Migrator().HasTable("revoked_tokens") {
		t.Skip("tabel revoked_tokens belum di-migrate; jalankan make migrate-up")
	}

	return db
}

// newPostgresNoJanitor membuat instance Postgres langsung tanpa menjalankan
// goroutine janitor (yang di-start oleh NewPostgres). Untuk unit integration
// kita tidak ingin janitor berjalan liar selama test.
func newPostgresNoJanitor(db *gorm.DB) *Postgres {
	return &Postgres{db: db}
}

// cleanup menghapus baris jti tertentu agar tabel revoked_tokens tidak kotor.
func cleanup(t *testing.T, db *gorm.DB, jtis ...string) {
	t.Helper()
	if len(jtis) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM revoked_tokens WHERE jti IN ?", jtis).Error; err != nil {
		t.Logf("peringatan: gagal membersihkan baris uji: %v", err)
	}
}

// TestPostgresRevokeThenIsRevoked memverifikasi jalur utama: setelah Revoke
// dengan exp di masa depan, IsRevoked mengembalikan true.
func TestPostgresRevokeThenIsRevoked(t *testing.T) {
	db := openTestDB(t)
	dl := newPostgresNoJanitor(db)

	jti := "test-" + uuid.NewString()
	t.Cleanup(func() { cleanup(t, db, jti) })

	dl.Revoke(jti, time.Now().Add(time.Hour))

	if !dl.IsRevoked(jti) {
		t.Errorf("IsRevoked(%q) = false, mau true setelah Revoke exp di masa depan", jti)
	}
}

// TestPostgresIsRevokedFalseForExpired memastikan jti dengan exp di masa lalu
// dianggap TIDAK dicabut (token-nya sudah mati sendiri) — IsRevoked=false.
func TestPostgresIsRevokedFalseForExpired(t *testing.T) {
	db := openTestDB(t)
	dl := newPostgresNoJanitor(db)

	jti := "test-" + uuid.NewString()
	t.Cleanup(func() { cleanup(t, db, jti) })

	dl.Revoke(jti, time.Now().Add(-time.Hour)) // exp sudah lewat

	if dl.IsRevoked(jti) {
		t.Errorf("IsRevoked(%q) = true, mau false untuk exp di masa lalu", jti)
	}
}

// TestPostgresIsRevokedEmptyJTI memastikan jti kosong selalu false tanpa
// menyentuh DB (guard di awal fungsi).
func TestPostgresIsRevokedEmptyJTI(t *testing.T) {
	db := openTestDB(t)
	dl := newPostgresNoJanitor(db)

	if dl.IsRevoked("") {
		t.Error("IsRevoked(\"\") = true, mau false")
	}
}

// TestPostgresRevokeIdempotent memastikan Revoke dua kali untuk jti yang sama
// (upsert ON CONFLICT) tidak error dan hasil akhirnya tetap dicabut. Panggilan
// kedua memperpanjang exp — dibuktikan dengan exp masa lalu lalu masa depan.
func TestPostgresRevokeIdempotent(t *testing.T) {
	db := openTestDB(t)
	dl := newPostgresNoJanitor(db)

	jti := "test-" + uuid.NewString()
	t.Cleanup(func() { cleanup(t, db, jti) })

	// Revoke pertama: exp di masa lalu -> IsRevoked=false.
	dl.Revoke(jti, time.Now().Add(-time.Hour))
	if dl.IsRevoked(jti) {
		t.Fatalf("setelah revoke exp lalu: IsRevoked = true, mau false")
	}

	// Revoke kedua untuk jti yang sama: exp masa depan (upsert, bukan duplicate
	// key error) -> IsRevoked=true. Membuktikan idempotensi/upsert.
	dl.Revoke(jti, time.Now().Add(time.Hour))
	if !dl.IsRevoked(jti) {
		t.Errorf("setelah revoke kedua (upsert): IsRevoked = false, mau true")
	}

	// Pastikan hanya ada satu baris (upsert, bukan insert kedua).
	var count int64
	if err := db.Raw("SELECT count(*) FROM revoked_tokens WHERE jti = ?", jti).Scan(&count).Error; err != nil {
		t.Fatalf("gagal hitung baris: %v", err)
	}
	if count != 1 {
		t.Errorf("jumlah baris untuk jti = %d, mau 1 (upsert)", count)
	}
}
