package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/mailer"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// genericForgotMessage sengaja tidak membedakan email terdaftar atau tidak.
// Membedakannya akan mengubah endpoint ini menjadi oracle untuk memetakan
// alamat email mana yang punya akun (user enumeration).
const genericForgotMessage = "jika email terdaftar, tautan reset password sudah dikirim"

type PasswordResetHandler struct {
	userRepo  repository.UserRepository
	tokenRepo repository.PasswordResetRepository
	mailer    mailer.Mailer
	cfg       *config.Config
}

func NewPasswordResetHandler(
	userRepo repository.UserRepository,
	tokenRepo repository.PasswordResetRepository,
	m mailer.Mailer,
	cfg *config.Config,
) *PasswordResetHandler {
	return &PasswordResetHandler{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		mailer:    m,
		cfg:       cfg,
	}
}

// hashResetToken mengubah token mentah menjadi SHA-256 heksadesimal.
// Yang tersimpan di DB hanya hash ini; token mentah cuma ada di email penerima.
// SHA-256 (bukan bcrypt) sudah memadai karena tokennya sendiri 256-bit acak —
// tidak bisa ditebak dengan brute force, jadi tidak butuh key stretching.
func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newResetToken membangkitkan token acak 256-bit dalam base64 URL-safe supaya
// aman ditaruh di query string tautan email.
func newResetToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// POST /api/v1/auth/forgot-password
// ForgotPassword godoc
// @Summary      Minta tautan reset password
// @Description  Mengirim tautan reset ke email bila terdaftar. Response SELALU sama (`200`) baik email terdaftar maupun tidak, untuk mencegah user enumeration. Rate-limited.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload body ForgotPasswordRequest true "Email akun"
// @Success      200  {object} MessageResponse
// @Failure      422  {object} ErrorResponse
// @Failure      429  {object} ErrorResponse
// @Failure      503  {object} ErrorResponse
// @Router       /auth/forgot-password [post]
func (h *PasswordResetHandler) ForgotPassword(c *fiber.Ctx) error {
	var req ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "validation failed",
				"code":    "VALIDATION_ERROR",
				"details": validation.ToFieldErrors(err),
			},
		})
	}

	// Masalah konfigurasi, bukan sinyal soal keberadaan akun — aman ditolak
	// terang-terangan sebelum menyentuh DB.
	if !h.mailer.Enabled() {
		return fiber.NewError(http.StatusServiceUnavailable, "reset password belum tersedia: SMTP belum dikonfigurasi")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Bersihkan token kedaluwarsa sekalian; murah karena `expires_at` ter-index
	// dan menghindari perlunya goroutine janitor terpisah.
	if err := h.tokenRepo.DeleteExpired(ctx); err != nil {
		log.Warn().Err(err).Msg("gagal membersihkan token reset kedaluwarsa")
	}

	user, err := h.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Err(err).Msg("gagal mencari user saat forgot password")
		}
		// Email tak terdaftar (atau DB bermasalah) tetap dijawab generik.
		return c.JSON(fiber.Map{"data": MessageResponse{Message: genericForgotMessage}})
	}

	// Satu token hidup per user: token lama dibatalkan lebih dulu.
	if err := h.tokenRepo.InvalidateAllForUser(ctx, user.ID); err != nil {
		log.Error().Err(err).Str("user_id", user.ID.String()).Msg("gagal membatalkan token reset lama")
		return c.JSON(fiber.Map{"data": MessageResponse{Message: genericForgotMessage}})
	}

	raw, err := newResetToken()
	if err != nil {
		log.Error().Err(err).Msg("gagal membangkitkan token reset")
		return c.JSON(fiber.Map{"data": MessageResponse{Message: genericForgotMessage}})
	}

	token := models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: hashResetToken(raw),
		ExpiresAt: time.Now().Add(time.Duration(h.cfg.PasswordResetTTL) * time.Second),
	}
	if err := h.tokenRepo.Create(ctx, &token); err != nil {
		log.Error().Err(err).Str("user_id", user.ID.String()).Msg("gagal menyimpan token reset")
		return c.JSON(fiber.Map{"data": MessageResponse{Message: genericForgotMessage}})
	}

	link := h.cfg.AppFrontendURL + "/auth/reset-password?token=" + raw
	subject := "Reset password akun portfolio"
	body := "Halo " + user.Name + ",\n\n" +
		"Kami menerima permintaan reset password untuk akun ini.\n" +
		"Buka tautan berikut untuk membuat password baru:\n\n" +
		link + "\n\n" +
		"Tautan berlaku " + minutesLabel(h.cfg.PasswordResetTTL) + " dan hanya bisa dipakai sekali.\n" +
		"Abaikan email ini bila kamu tidak merasa memintanya.\n"

	// Kegagalan kirim TIDAK diubah jadi error response: membedakan sukses/gagal
	// di sini akan membocorkan bahwa email tersebut terdaftar. Operator melihat
	// kegagalannya lewat log.
	if err := h.mailer.Send(ctx, user.Email, subject, body); err != nil {
		log.Error().Err(err).Str("user_id", user.ID.String()).Msg("gagal mengirim email reset password")
	}

	return c.JSON(fiber.Map{"data": MessageResponse{Message: genericForgotMessage}})
}

// POST /api/v1/auth/reset-password
// ResetPassword godoc
// @Summary      Setel password baru dengan token reset
// @Description  Menukar token dari tautan email dengan password baru. Token hanya berlaku sekali dan punya masa kedaluwarsa. Catatan: sesi/JWT yang sudah terbit sebelum reset TIDAK ikut dicabut.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload body ResetPasswordRequest true "Token + password baru"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} ErrorResponse
// @Failure      422  {object} ErrorResponse
// @Router       /auth/reset-password [post]
func (h *PasswordResetHandler) ResetPassword(c *fiber.Ctx) error {
	var req ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "validation failed",
				"code":    "VALIDATION_ERROR",
				"details": validation.ToFieldErrors(err),
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := h.tokenRepo.FindValidByHash(ctx, hashResetToken(req.Token))
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Err(err).Msg("gagal mencari token reset")
		}
		// Tidak ketemu, sudah dipakai, atau kedaluwarsa — semuanya dijawab sama
		// agar tidak membocorkan token mana yang pernah ada.
		return fiber.NewError(http.StatusBadRequest, "token reset tidak valid atau sudah kedaluwarsa")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("gagal hashing password baru")
		return fiber.NewError(http.StatusInternalServerError, "gagal menyetel password baru")
	}

	if err := h.userRepo.UpdatePassword(ctx, token.UserID, string(hashed)); err != nil {
		log.Error().Err(err).Str("user_id", token.UserID.String()).Msg("gagal memperbarui password")
		return fiber.NewError(http.StatusInternalServerError, "gagal menyetel password baru")
	}

	// Token dipakai → langsung dimatikan, berikut sisa token lain milik user
	// yang sama supaya tautan lama di inbox tidak bisa dipakai lagi.
	if err := h.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		log.Error().Err(err).Msg("gagal menandai token reset terpakai")
	}
	if err := h.tokenRepo.InvalidateAllForUser(ctx, token.UserID); err != nil {
		log.Error().Err(err).Msg("gagal membatalkan sisa token reset")
	}

	return c.JSON(fiber.Map{
		"data": MessageResponse{Message: "password berhasil diperbarui, silakan login dengan password baru"},
	})
}

// minutesLabel memformat TTL detik menjadi label menit/jam yang enak dibaca
// di badan email.
func minutesLabel(seconds int) string {
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Second).String()
}
