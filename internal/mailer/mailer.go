// Package mailer membungkus pengiriman email transaksional (mis. tautan reset
// password). Interface-nya sengaja minimal agar handler mudah di-test tanpa
// menyentuh jaringan.
package mailer

import (
	"context"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/rs/zerolog/log"
)

// Mailer mengirim email teks polos.
type Mailer interface {
	// Enabled memberi tahu apakah pengiriman benar-benar tersedia. Handler
	// memakainya untuk menolak lebih awal (503) alih-alih berpura-pura sukses.
	Enabled() bool
	// Send mengirim satu email. Implementasi wajib menghormati ctx.
	Send(ctx context.Context, to, subject, body string) error
}

// New memilih implementasi berdasarkan konfigurasi:
//
//   - SMTP terisi                  → kirim sungguhan lewat SMTP.
//   - SMTP kosong, non-produksi    → logMailer: isi email ditulis ke log supaya
//     developer bisa menyalin tautan reset tanpa menyiapkan SMTP.
//   - SMTP kosong, produksi        → disabledMailer: menolak mengirim. Sengaja
//     TIDAK jatuh ke logMailer karena itu berarti menulis tautan reset password
//     (setara kredensial sementara) ke log produksi.
func New(cfg *config.Config) Mailer {
	if cfg.MailerConfigured() {
		return newSMTPMailer(cfg)
	}

	if cfg.IsProduction() {
		log.Error().Msg("SMTP belum dikonfigurasi di produksi: pengiriman email dinonaktifkan (reset password akan menolak dengan 503)")
		return disabledMailer{}
	}

	log.Warn().Msg("SMTP belum dikonfigurasi: email hanya akan ditulis ke log (mode pengembangan)")
	return logMailer{}
}

// logMailer menulis email ke log alih-alih mengirimnya. HANYA untuk
// pengembangan lokal — lihat catatan di New.
type logMailer struct{}

func (logMailer) Enabled() bool { return true }

func (logMailer) Send(_ context.Context, to, subject, body string) error {
	log.Info().
		Str("to", to).
		Str("subject", subject).
		Str("body", body).
		Msg("[DEV] email tidak dikirim (SMTP kosong), isi ditulis ke log")
	return nil
}

// disabledMailer menolak semua pengiriman. Dipakai di produksi saat SMTP belum
// dikonfigurasi, supaya kegagalan terlihat jelas dan tidak senyap.
type disabledMailer struct{}

func (disabledMailer) Enabled() bool { return false }

func (disabledMailer) Send(_ context.Context, _, _, _ string) error {
	return ErrNotConfigured
}
