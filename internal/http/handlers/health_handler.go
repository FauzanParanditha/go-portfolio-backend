package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/response"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

const (
	// defaultReadyPingTimeout membatasi lama satu ping database.
	defaultReadyPingTimeout = 2 * time.Second

	// defaultReadyMemoTTL adalah umur hasil readiness yang di-memo. Selama
	// jendela ini semua request dijawab dari memo, sehingga database menerima
	// paling banyak satu ping per jendela dari jalur publik ini, berapa pun
	// jumlah pengunjungnya.
	defaultReadyMemoTTL = 5 * time.Second

	// readyUnavailableMessage sengaja generik: keputusan pemilik produk, status
	// database tidak dirinci ke publik. Detail aslinya hanya ada di log server.
	readyUnavailableMessage = "layanan sedang terganggu"
)

var (
	errPingerTidakAda    = errors.New("pinger database tidak dikonfigurasi")
	errPingMasihBerjalan = errors.New("ping sebelumnya melewati batas waktu dan belum selesai")
)

// Pinger adalah antarmuka kecil yang dibutuhkan pemeriksaan readiness.
// Dipenuhi oleh *sql.DB; di test diganti tiruan sehingga tidak butuh database.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// HealthOptions menyuntikkan batas waktu, umur memo, dan sumber waktu. Nilai
// nol berarti memakai default produksi (2 detik, 5 detik, time.Now). Dipakai
// test agar tidak perlu menunggu waktu sungguhan.
type HealthOptions struct {
	PingTimeout time.Duration
	MemoTTL     time.Duration
	Now         func() time.Time
}

// HealthHandler melayani endpoint status publik di bawah /api/v1. Route root
// /healthz dan /readyz (monitoring/orkestrasi) TIDAK lewat handler ini.
type HealthHandler struct {
	pinger      Pinger
	pingTimeout time.Duration
	memoTTL     time.Duration
	now         func() time.Time

	// mu menjaga seluruh field di bawahnya dan DIPEGANG selama ping berjalan.
	// Request yang datang bersamaan saat memo kedaluwarsa mengantre di sini;
	// begitu mendapat giliran, mereka menemukan memo yang baru diisi dan tidak
	// melakukan ping lagi.
	mu        sync.Mutex
	checked   bool
	checkedAt time.Time
	lastErr   error
	// pending adalah kanal hasil ping yang melewati batas waktu tetapi
	// goroutine-nya belum kembali (driver mengabaikan pembatalan context).
	pending chan error
}

// NewHealthHandler membuat handler dengan nilai default produksi. pinger boleh
// nil (mis. router dirakit tanpa database): readiness lalu selalu 503.
func NewHealthHandler(pinger Pinger) *HealthHandler {
	return NewHealthHandlerWithOptions(pinger, HealthOptions{})
}

// NewHealthHandlerWithOptions sama seperti NewHealthHandler dengan batas waktu,
// umur memo, dan sumber waktu yang bisa disuntik.
func NewHealthHandlerWithOptions(pinger Pinger, opts HealthOptions) *HealthHandler {
	if opts.PingTimeout <= 0 {
		opts.PingTimeout = defaultReadyPingTimeout
	}
	if opts.MemoTTL <= 0 {
		opts.MemoTTL = defaultReadyMemoTTL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &HealthHandler{
		pinger:      pinger,
		pingTimeout: opts.PingTimeout,
		memoTTL:     opts.MemoTTL,
		now:         opts.Now,
	}
}

// GET /api/v1/healthz
// Healthz godoc
// @Summary      Status hidup layanan
// @Description  Liveness untuk tampilan status di situs publik: proses API hidup dan menjawab. Tidak menyentuh database. Rate limit 60 request/menit per IP, satu jatah bersama dengan `/readyz`. Respons membawa `Cache-Control: no-store`.
// @Tags         health
// @Produce      json
// @Success      200  {object} HealthResponse
// @Failure      429  {object} ErrorResponse "Terlalu banyak permintaan (60/menit per IP, jatah bersama dengan /readyz)"
// @Router       /healthz [get]
func (h *HealthHandler) Healthz(c *fiber.Ctx) error {
	return response.OK(c, HealthStatus{Status: "ok"})
}

// GET /api/v1/readyz
// Readyz godoc
// @Summary      Status kesiapan layanan
// @Description  Readiness untuk tampilan status di situs publik. Hasil pemeriksaan di-memo 5 detik di server, jadi status bisa tertinggal paling lama 5 detik. Bila layanan tidak siap, balasannya 503 dengan pesan generik tanpa rincian penyebab. Rate limit 60 request/menit per IP, satu jatah bersama dengan `/healthz`. Respons membawa `Cache-Control: no-store`.
// @Tags         health
// @Produce      json
// @Success      200  {object} HealthResponse
// @Failure      429  {object} ErrorResponse "Terlalu banyak permintaan (60/menit per IP, jatah bersama dengan /healthz)"
// @Failure      503  {object} ErrorResponse "Layanan sedang terganggu (kode SERVICE_UNAVAILABLE)"
// @Router       /readyz [get]
func (h *HealthHandler) Readyz(c *fiber.Ctx) error {
	if err := h.checkReady(); err != nil {
		// Penyebab asli sudah di-log oleh checkReady; ke publik hanya pesan generik.
		return fiber.NewError(fiber.StatusServiceUnavailable, readyUnavailableMessage)
	}
	return response.OK(c, HealthStatus{Status: "ready"})
}

// checkReady mengembalikan hasil readiness dari memo bila masih segar, atau
// melakukan satu ping lalu menyimpan hasilnya (sukses MAUPUN gagal, supaya
// database yang sedang bermasalah tidak ikut dibanjiri ping).
func (h *HealthHandler) checkReady() error {
	if h.pinger == nil {
		return errPingerTidakAda
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.checked && h.now().Sub(h.checkedAt) < h.memoTTL {
		return h.lastErr
	}

	err := h.ping()
	// Dicap SETELAH ping selesai: jarak antara akhir satu ping dan awal ping
	// berikutnya tidak pernah kurang dari umur memo, walau ping-nya lambat.
	h.checked, h.checkedAt, h.lastErr = true, h.now(), err
	if err != nil {
		// Di-log sekali per ping, bukan sekali per request yang dijawab memo.
		log.Error().Err(err).Msg("readiness: ping database gagal")
	}
	return err
}

// ping menjalankan satu PingContext dengan batas waktu. Harus dipanggil dengan
// h.mu terpegang.
//
// Ping dijalankan di goroutine terpisah agar batas waktu tetap berlaku walau
// driver mengabaikan pembatalan context. Bila itu terjadi, goroutine-nya
// dicatat di h.pending dan TIDAK ADA ping baru sebelum ia kembali — jadi paling
// banyak satu ping berjalan pada satu waktu, dan goroutine tidak menumpuk.
func (h *HealthHandler) ping() error {
	if h.pending != nil {
		select {
		case <-h.pending:
			// Ping lama akhirnya selesai; hasilnya sudah basi, dibuang.
			h.pending = nil
		default:
			return errPingMasihBerjalan
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.pingTimeout)
	defer cancel()

	done := make(chan error, 1) // ber-buffer: goroutine tidak pernah tersangkut saat mengirim
	go func() {
		// recover middleware tidak menjangkau goroutine ini; panic di driver
		// tidak boleh menjatuhkan seluruh proses dari endpoint publik.
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic saat ping database: %v", r)
			}
		}()
		done <- h.pinger.PingContext(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		h.pending = done
		return ctx.Err()
	}
}
