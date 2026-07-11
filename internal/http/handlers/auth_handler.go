package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/denylist"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userRepo repository.UserRepository
	cfg      *config.Config
	denylist denylist.Denylist
}

func NewAuthHandler(userRepo repository.UserRepository, cfg *config.Config, dl denylist.Denylist) *AuthHandler {
	return &AuthHandler{
		userRepo: userRepo,
		cfg:      cfg,
		denylist: dl,
	}
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"tokenType"`
	ExpiresIn int    `json:"expiresIn"` // detik
}

type JWTCustomClaims struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// accessTokenCookieName adalah nama cookie HttpOnly yang menyimpan JWT untuk
// klien browser. API client non-browser tetap bisa pakai header Authorization.
const accessTokenCookieName = "access_token"

// setAuthCookie menulis cookie `access_token` yang berisi JWT.
// Atribut: HttpOnly, Path=/, SameSite=Lax, Max-Age=JWT_EXPIRES_IN detik.
// Flag Secure hanya dipasang di produksi agar tetap jalan di http://localhost saat dev.
func (h *AuthHandler) setAuthCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     accessTokenCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   h.cfg.JWTExpiresIn,
		HTTPOnly: true,
		Secure:   h.cfg.IsProduction(),
		SameSite: "Lax",
	})
}

// clearAuthCookie menghapus cookie `access_token` dengan menulis cookie kedaluwarsa
// (nilai kosong, Expires di masa lalu / MaxAge negatif). Atribut Path/Secure/SameSite
// dijaga konsisten dengan setAuthCookie agar browser benar-benar menimpanya.
func (h *AuthHandler) clearAuthCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     accessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   h.cfg.IsProduction(),
		SameSite: "Lax",
	})
}

// POST /api/v1/auth/login
// Login godoc
// @Summary      Login admin
// @Description  Authenticate admin and return JWT token. Selain body JSON, server juga men-set cookie HttpOnly `access_token` (SameSite=Lax, Secure di produksi) yang dipakai klien browser. API client non-browser bisa mengabaikan cookie dan memakai field `token` di body via header Authorization.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      LoginRequest  true  "Login data"
// @Success      200      {object}  LoginResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		fieldErrors := validation.ToFieldErrors(err)
		return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "validation failed",
				"code":    "VALIDATION_ERROR",
				"details": fieldErrors,
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		log.Warn().Err(err).Str("email", req.Email).Msg("login failed: user not found")
		// jangan bocorkan info yang terlalu detail
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		log.Warn().Err(err).Str("email", req.Email).Msg("login failed: wrong password")
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	// Generate JWT (iat/exp segar) lewat helper bersama.
	signed, err := h.issueToken(user.ID.String(), user.Role)
	if err != nil {
		log.Error().Err(err).Msg("failed to sign JWT")
		return fiber.NewError(http.StatusInternalServerError, "failed to generate token")
	}

	// Set cookie HttpOnly untuk klien browser; body token tetap dikembalikan
	// untuk API client non-browser & test.
	h.setAuthCookie(c, signed)

	// Envelope diseragamkan: semua response sukses dibungkus { "data": ... }.
	return c.JSON(fiber.Map{
		"data": LoginResponse{
			Token:     signed,
			TokenType: "Bearer",
			ExpiresIn: h.cfg.JWTExpiresIn,
		},
	})
}

// issueToken membuat JWT HS256 baru (iat/exp segar) untuk userID+role tertentu.
// Dipakai bersama oleh Login dan Refresh agar bentuk token konsisten.
func (h *AuthHandler) issueToken(userID, role string) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(h.cfg.JWTExpiresIn) * time.Second)

	claims := JWTCustomClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(), // jti — dipakai untuk revocation saat logout
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.cfg.JWTSecret))
}

// POST /api/v1/auth/refresh
// Refresh godoc
// @Summary      Refresh JWT token
// @Description  Validasi token saat ini (via AuthJWT, menerima token dari header Authorization ATAU cookie `access_token`) lalu terbitkan token baru (iat/exp segar) dengan userId+role yang sama. Juga men-set ulang cookie HttpOnly `access_token`. Stateless, tanpa refresh-token store.
// @Tags         auth
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  LoginResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /auth/refresh [post]
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	// AuthJWT (middleware) sudah memvalidasi token & menyimpan klaim ke Locals.
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	role, _ := c.Locals("user_role").(string)

	signed, err := h.issueToken(userID, role)
	if err != nil {
		log.Error().Err(err).Msg("failed to sign refreshed JWT")
		return fiber.NewError(http.StatusInternalServerError, "failed to generate token")
	}

	// Set ulang cookie HttpOnly dengan token segar.
	h.setAuthCookie(c, signed)

	return c.JSON(fiber.Map{
		"data": LoginResponse{
			Token:     signed,
			TokenType: "Bearer",
			ExpiresIn: h.cfg.JWTExpiresIn,
		},
	})
}

// POST /api/v1/auth/logout
// Logout godoc
// @Summary      Logout
// @Description  Menghapus cookie HttpOnly `access_token` di sisi browser DAN mencabut token saat ini (berdasarkan jti) lewat denylist sehingga token langsung tidak berlaku walau belum `exp`. Bersifat PUBLIK (tanpa middleware auth) agar user dengan token kedaluwarsa tetap bisa membersihkan cookie; token yang sudah invalid/kedaluwarsa cukup diabaikan.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// Cabut token saat ini (jika valid) berdasarkan jti agar logout langsung
	// mematikan token, tidak menunggu sampai exp.
	if h.denylist != nil {
		if jti, exp, ok := h.parseTokenForRevoke(c); ok {
			h.denylist.Revoke(jti, exp)
		}
	}

	h.clearAuthCookie(c)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"message": "logged out",
		},
	})
}

// parseTokenForRevoke membaca token dari header Authorization ATAU cookie
// `access_token`, mem-parse & memvalidasinya (HS256, secret yang sama), lalu
// mengembalikan jti + exp. ok=false bila token tidak ada / tidak valid / tanpa jti
// (tidak ada yang perlu dicabut).
func (h *AuthHandler) parseTokenForRevoke(c *fiber.Ctx) (jti string, exp time.Time, ok bool) {
	var tokenStr string
	if authHeader := c.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			tokenStr = parts[1]
		}
	} else if cookie := c.Cookies(accessTokenCookieName); cookie != "" {
		tokenStr = cookie
	}
	if tokenStr == "" {
		return "", time.Time{}, false
	}

	claims := &JWTCustomClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.ID == "" || claims.ExpiresAt == nil {
		return "", time.Time{}, false
	}
	return claims.ID, claims.ExpiresAt.Time, true
}
