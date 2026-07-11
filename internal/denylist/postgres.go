package denylist

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RevokedToken adalah baris tabel `revoked_tokens` (denylist persisten).
// Skema dikelola lewat migration Atlas, bukan AutoMigrate.
type RevokedToken struct {
	JTI       string    `gorm:"column:jti;primaryKey"`
	Exp       time.Time `gorm:"column:exp"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName memaksa nama tabel ke `revoked_tokens`.
func (RevokedToken) TableName() string {
	return "revoked_tokens"
}

// Postgres adalah Denylist persisten berbasis PostgreSQL. Berbeda dari Memory,
// state-nya bertahan saat proses restart dan dapat dibagikan antar-instance.
type Postgres struct {
	db *gorm.DB
}

// NewPostgres membuat denylist berbasis Postgres dan menjalankan janitor yang
// membersihkan entri kedaluwarsa secara periodik (meniru pola Memory.janitor).
func NewPostgres(db *gorm.DB) *Postgres {
	d := &Postgres{db: db}
	go d.janitor(10 * time.Minute)
	return d
}

// Revoke menandai jti sebagai dicabut sampai exp. Memakai upsert
// (ON CONFLICT DO UPDATE) sehingga aman dipanggil berulang untuk jti yang sama.
// jti kosong diabaikan.
func (d *Postgres) Revoke(jti string, exp time.Time) {
	if jti == "" {
		return
	}

	// Interface Denylist tidak membawa context, jadi buat context dengan timeout
	// sendiri agar upsert tidak menggantung tanpa batas bila Postgres hang.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "jti"}},
			DoUpdates: clause.AssignmentColumns([]string{"exp"}),
		}).
		Create(&RevokedToken{JTI: jti, Exp: exp}).Error
	if err != nil {
		// Gagal/timeout mencabut hanya di-log; caller (logout) tetap sukses agar
		// UX tidak terganggu. Kegagalan langka ini berarti token tetap berlaku
		// sampai exp — risiko terbatas.
		log.Error().Err(err).Str("jti", jti).Msg("gagal mencabut token ke denylist")
	}
}

// IsRevoked mengembalikan true jika jti masih dicabut dan belum lewat exp.
// jti kosong => false.
//
// KEPUTUSAN FAIL-OPEN (disengaja): bila query error ATAU timeout (mis. DB down
// atau Postgres hang), fungsi me-log lalu mengembalikan false alih-alih true.
// Sebab IsRevoked dipanggil di jalur panas SETIAP request terautentikasi; bila
// error dianggap "dicabut" (fail-closed), satu gangguan DB akan menolak SEMUA
// request auth sekaligus — sebuah DoS. Kita memilih ketersediaan: token yang
// dicabut mungkin sesaat masih diterima saat DB bermasalah, tapi seluruh sistem
// tidak lumpuh.
//
// Timeout singkat wajib di sini: tanpa deadline, Postgres yang hang (bukan
// error) membuat request auth menumpuk tanpa batas. Timeout diperlakukan sama
// seperti error → fail-open.
func (d *Postgres) IsRevoked(jti string) bool {
	if jti == "" {
		return false
	}

	// Interface Denylist tidak membawa context, jadi buat context dengan timeout
	// singkat sendiri agar cek di jalur panas tidak menggantung saat DB hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var exists bool
	err := d.db.WithContext(ctx).
		Raw(
			"SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = ? AND exp > now())",
			jti,
		).Scan(&exists).Error
	if err != nil {
		log.Error().Err(err).Str("jti", jti).
			Msg("gagal/timeout cek denylist token; fail-open (dianggap tidak dicabut)")
		return false
	}

	return exists
}

// janitor membersihkan entri yang sudah lewat exp setiap interval.
func (d *Postgres) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		// Timeout agar query DELETE tidak menggantung tanpa batas bila DB hang;
		// error/timeout cukup di-log, janitor lanjut pada tick berikutnya.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := d.db.WithContext(ctx).
			Exec("DELETE FROM revoked_tokens WHERE exp < now()").Error
		cancel()
		if err != nil {
			log.Warn().Err(err).Msg("janitor denylist gagal/timeout membersihkan entri kedaluwarsa")
		}
	}
}
