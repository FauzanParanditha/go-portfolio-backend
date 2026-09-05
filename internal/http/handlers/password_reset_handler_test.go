package handlers_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// resetCfg adalah config uji untuk alur reset password: frontend URL dipakai
// menyusun tautan email, TTL menentukan masa berlaku token.
func resetCfg() *config.Config {
	return &config.Config{
		AppEnv:           "development",
		AppFrontendURL:   "https://app.example.com",
		PasswordResetTTL: 3600,
	}
}

// sha256Hex meniru cara handler menyimpan token (hash, bukan token mentah)
// sehingga test bisa mencocokkan tanpa mengekspos helper internal.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newResetApp merakit app + handler reset password dengan fake yang diberikan.
func newResetApp(
	users *fakeUserRepo,
	tokens *fakePasswordResetRepo,
	m *fakeMailer,
) *fiber.App {
	app := newTestApp()
	h := handlers.NewPasswordResetHandler(users, tokens, m, resetCfg())
	app.Post("/auth/forgot-password", h.ForgotPassword)
	app.Post("/auth/reset-password", h.ResetPassword)
	return app
}

// TestForgotPasswordKirimTautan memastikan alur bahagia: token disimpan dalam
// bentuk HASH, dan email berisi tautan dengan token MENTAH.
func TestForgotPasswordKirimTautan(t *testing.T) {
	userID := uuid.New()
	users := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, email string) (*models.User, error) {
			return &models.User{ID: userID, Name: "Admin", Email: email, Role: "admin"}, nil
		},
	}

	var saved *models.PasswordResetToken
	tokens := &fakePasswordResetRepo{
		createFn: func(_ context.Context, tk *models.PasswordResetToken) error {
			saved = tk
			return nil
		},
	}
	mail := &fakeMailer{enabled: true}

	app := newResetApp(users, tokens, mail)
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/forgot-password", `{"email":"admin@example.com"}`))

	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body: %s", status, body)
	}
	if mail.sendCount != 1 {
		t.Fatalf("email terkirim %d kali, mau 1", mail.sendCount)
	}
	if mail.sentTo != "admin@example.com" {
		t.Fatalf("email dikirim ke %q, mau admin@example.com", mail.sentTo)
	}
	if saved == nil {
		t.Fatal("token tidak disimpan ke repository")
	}
	if saved.UserID != userID {
		t.Fatalf("token disimpan untuk user %s, mau %s", saved.UserID, userID)
	}

	// Ambil token mentah dari tautan di badan email, lalu pastikan yang
	// tersimpan adalah hash-nya — bukan token itu sendiri.
	const marker = "https://app.example.com/auth/reset-password?token="
	idx := strings.Index(mail.sentBody, marker)
	if idx < 0 {
		t.Fatalf("badan email tidak memuat tautan reset. body:\n%s", mail.sentBody)
	}
	raw := strings.Fields(mail.sentBody[idx+len(marker):])[0]
	if raw == "" {
		t.Fatal("token pada tautan kosong")
	}
	if saved.TokenHash == raw {
		t.Fatal("token mentah tersimpan di DB; seharusnya hanya hash-nya")
	}
	if saved.TokenHash != sha256Hex(raw) {
		t.Fatalf("token_hash tidak cocok dengan SHA-256 token pada tautan")
	}
	if !saved.ExpiresAt.After(time.Now()) {
		t.Fatalf("expires_at %v sudah lewat saat dibuat", saved.ExpiresAt)
	}
}

// TestForgotPasswordEmailTidakTerdaftar menjaga sifat anti-enumeration:
// email asing dijawab persis sama dengan email terdaftar, tanpa kirim email.
func TestForgotPasswordEmailTidakTerdaftar(t *testing.T) {
	users := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	mail := &fakeMailer{enabled: true}

	app := newResetApp(users, &fakePasswordResetRepo{}, mail)
	statusAsing, bodyAsing := doJSON(t, app, postJSON(http.MethodPost, "/auth/forgot-password", `{"email":"bukan-user@example.com"}`))

	if statusAsing != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (tidak boleh membocorkan ketiadaan akun)", statusAsing)
	}
	if mail.sendCount != 0 {
		t.Fatalf("email terkirim %d kali untuk akun yang tidak ada, mau 0", mail.sendCount)
	}

	// Bandingkan dengan respons untuk email yang TERDAFTAR: harus identik.
	usersAda := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, email string) (*models.User, error) {
			return &models.User{ID: uuid.New(), Name: "Admin", Email: email}, nil
		},
	}
	appAda := newResetApp(usersAda, &fakePasswordResetRepo{}, &fakeMailer{enabled: true})
	statusAda, bodyAda := doJSON(t, appAda, postJSON(http.MethodPost, "/auth/forgot-password", `{"email":"admin@example.com"}`))

	if statusAda != statusAsing {
		t.Fatalf("status berbeda: terdaftar=%d, asing=%d", statusAda, statusAsing)
	}
	if string(bodyAda) != string(bodyAsing) {
		t.Fatalf("body berbeda → jadi oracle enumerasi.\nterdaftar: %s\nasing:     %s", bodyAda, bodyAsing)
	}
}

// TestForgotPasswordMailerNonaktif: masalah konfigurasi ditolak terang-terangan
// (503), bukan dijawab sukses palsu.
func TestForgotPasswordMailerNonaktif(t *testing.T) {
	users := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			t.Fatal("repository tidak boleh disentuh saat mailer nonaktif")
			return nil, nil
		},
	}

	app := newResetApp(users, &fakePasswordResetRepo{}, &fakeMailer{enabled: false})
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/forgot-password", `{"email":"admin@example.com"}`))

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503. body: %s", status, body)
	}
}

// TestForgotPasswordValidasiEmail memastikan email tak valid ditolak 422.
func TestForgotPasswordValidasiEmail(t *testing.T) {
	app := newResetApp(&fakeUserRepo{}, &fakePasswordResetRepo{}, &fakeMailer{enabled: true})
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/forgot-password", `{"email":"bukan-email"}`))

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422. body: %s", status, body)
	}
	m := decode(t, body)
	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope error tidak ditemukan: %s", body)
	}
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Fatalf("code = %v, mau VALIDATION_ERROR", errObj["code"])
	}
}

// TestResetPasswordSukses memverifikasi password benar-benar diganti (hash baru
// cocok dengan password baru) dan token langsung dimatikan.
func TestResetPasswordSukses(t *testing.T) {
	userID := uuid.New()
	tokenID := uuid.New()
	const rawToken = "token-mentah-uji"
	const newPassword = "password-baru-123"

	var gotHash string
	var gotUserID uuid.UUID
	users := &fakeUserRepo{
		updatePasswordFn: func(_ context.Context, id uuid.UUID, hashed string) error {
			gotUserID, gotHash = id, hashed
			return nil
		},
	}

	markedUsed := false
	invalidated := false
	tokens := &fakePasswordResetRepo{
		findValidByHashFn: func(_ context.Context, hash string) (*models.PasswordResetToken, error) {
			if hash != sha256Hex(rawToken) {
				t.Fatalf("handler mencari hash %q, mau SHA-256 dari token mentah", hash)
			}
			return &models.PasswordResetToken{
				ID:        tokenID,
				UserID:    userID,
				TokenHash: hash,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
		markUsedFn: func(_ context.Context, id uuid.UUID) error {
			if id != tokenID {
				t.Fatalf("MarkUsed dipanggil untuk token %s, mau %s", id, tokenID)
			}
			markedUsed = true
			return nil
		},
		invalidateAllForUserFn: func(_ context.Context, id uuid.UUID) error {
			if id != userID {
				t.Fatalf("InvalidateAllForUser untuk user %s, mau %s", id, userID)
			}
			invalidated = true
			return nil
		},
	}

	app := newResetApp(users, tokens, &fakeMailer{enabled: true})
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/reset-password",
		`{"token":"`+rawToken+`","password":"`+newPassword+`"}`))

	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body: %s", status, body)
	}
	if gotUserID != userID {
		t.Fatalf("password diperbarui untuk user %s, mau %s", gotUserID, userID)
	}
	if gotHash == newPassword {
		t.Fatal("password disimpan sebagai teks polos")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotHash), []byte(newPassword)); err != nil {
		t.Fatalf("hash tersimpan tidak cocok dengan password baru: %v", err)
	}
	if !markedUsed {
		t.Error("token tidak ditandai terpakai; tautan bisa dipakai ulang")
	}
	if !invalidated {
		t.Error("sisa token milik user tidak dibatalkan")
	}
}

// TestResetPasswordTokenTidakValid: token kedaluwarsa/terpakai/tak ada semuanya
// dijawab 400 yang sama.
func TestResetPasswordTokenTidakValid(t *testing.T) {
	users := &fakeUserRepo{
		updatePasswordFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			t.Fatal("password tidak boleh diubah dengan token tidak valid")
			return nil
		},
	}
	tokens := &fakePasswordResetRepo{
		findValidByHashFn: func(_ context.Context, _ string) (*models.PasswordResetToken, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newResetApp(users, tokens, &fakeMailer{enabled: true})
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/reset-password",
		`{"token":"kedaluwarsa","password":"password-baru-123"}`))

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body: %s", status, body)
	}
}

// TestResetPasswordTerlaluPendek menjaga kebijakan panjang minimum password.
func TestResetPasswordTerlaluPendek(t *testing.T) {
	tokens := &fakePasswordResetRepo{
		findValidByHashFn: func(_ context.Context, _ string) (*models.PasswordResetToken, error) {
			t.Fatal("token tidak boleh dicari sebelum validasi password lolos")
			return nil, nil
		},
	}

	app := newResetApp(&fakeUserRepo{}, tokens, &fakeMailer{enabled: true})
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/auth/reset-password",
		`{"token":"apa-saja","password":"pendek"}`))

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422. body: %s", status, body)
	}
}
