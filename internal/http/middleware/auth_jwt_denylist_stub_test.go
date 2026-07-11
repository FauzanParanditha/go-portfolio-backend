package middleware_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
)

// stubDenylist adalah implementasi Denylist in-test (tanpa DB). IsRevoked
// mengembalikan true hanya untuk jti yang ada di set `revoked`. Ini membuktikan
// middleware AuthJWT menghormati keputusan revocation apa pun sumbernya, tanpa
// bergantung pada implementasi Memory/Postgres.
type stubDenylist struct {
	revoked map[string]bool
}

func (s *stubDenylist) Revoke(jti string, _ time.Time) { s.revoked[jti] = true }

func (s *stubDenylist) IsRevoked(jti string) bool { return s.revoked[jti] }

// TestAuthJWTHonorsDenylistStub memastikan AuthJWT:
//   - MENOLAK (401) token valid yang jti-nya dilaporkan dicabut oleh stub, dan
//   - MELOLOSKAN (bukan 401) token valid yang jti-nya tidak dicabut.
//
// Sepenuhnya DB-free: memakai stub Denylist, JWT HS256 bertanda tangan, dan
// app.Test — mengikuti pola require_role_test.go.
func TestAuthJWTHonorsDenylistStub(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}

	const revokedJTI = "jti-dicabut-oleh-stub"
	const liveJTI = "jti-masih-hidup"

	dl := &stubDenylist{revoked: map[string]bool{revokedJTI: true}}
	app := newRevokeApp(cfg, dl)

	// Token dengan jti yang dicabut -> 401.
	revokedToken := signTokenWithJTI(t, cfg.JWTSecret, revokedJTI)
	if code := doGet(t, app, revokedToken); code != http.StatusUnauthorized {
		t.Errorf("jti dicabut: status = %d, mau 401", code)
	}

	// Token dengan jti yang tidak dicabut -> bukan 401 (lolos, 200).
	liveToken := signTokenWithJTI(t, cfg.JWTSecret, liveJTI)
	if code := doGet(t, app, liveToken); code == http.StatusUnauthorized {
		t.Errorf("jti tidak dicabut: status = %d, tidak boleh 401", code)
	} else if code != http.StatusOK {
		t.Errorf("jti tidak dicabut: status = %d, mau 200", code)
	}
}
