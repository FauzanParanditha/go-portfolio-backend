package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Test di berkas ini DB-free: Pinger diganti tiruan, dan waktu (jam + batas
// waktu ping) disuntik lewat HealthOptions sehingga tidak ada yang menunggu
// 2 detik / 5 detik sungguhan.

// pingerTiruan menghitung pemanggilan PingContext dan menjalankan fn bila ada.
type pingerTiruan struct {
	calls atomic.Int32
	fn    func(ctx context.Context) error
}

func (p *pingerTiruan) PingContext(ctx context.Context) error {
	p.calls.Add(1)
	if p.fn != nil {
		return p.fn(ctx)
	}
	return nil
}

// jamTiruan adalah sumber waktu yang hanya maju bila diminta. Aman dipakai
// dari banyak goroutine (handler membacanya saat test memajukannya).
type jamTiruan struct {
	mu  sync.Mutex
	now time.Time
}

func newJamTiruan() *jamTiruan {
	return &jamTiruan{now: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)}
}

func (j *jamTiruan) Now() time.Time {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.now
}

func (j *jamTiruan) Maju(d time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.now = j.now.Add(d)
}

func newHealthApp(pinger handlers.Pinger, opts handlers.HealthOptions) *fiber.App {
	h := handlers.NewHealthHandlerWithOptions(pinger, opts)
	app := newTestApp()
	app.Get("/api/v1/healthz", h.Healthz)
	app.Get("/api/v1/readyz", h.Readyz)
	return app
}

// getStatus menjalankan GET lewat app.Test dengan batas 5 detik (bukan -1),
// supaya handler yang tersangkut menggagalkan test alih-alih menggantungnya.
// Tidak memanggil t.Fatal agar aman dipanggil dari goroutine lain.
func getStatus(app *fiber.App, path string) (int, string, error) {
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), 5000)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

func mustGet(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	status, body, err := getStatus(app, path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return status, body
}

func TestHealthzTidakMenyentuhPinger(t *testing.T) {
	pinger := &pingerTiruan{}
	app := newHealthApp(pinger, handlers.HealthOptions{})

	status, body := mustGet(t, app, "/api/v1/healthz")
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, body)
	}
	if body != `{"data":{"status":"ok"}}` {
		t.Errorf("body = %s, mau {\"data\":{\"status\":\"ok\"}}", body)
	}
	if n := pinger.calls.Load(); n != 0 {
		t.Errorf("liveness memanggil Pinger %d kali, mau 0", n)
	}
}

// N request beruntun dalam jendela memo → tepat satu ping. Setelah jendela
// lewat → ping lagi. Waktu disuntik, tidak ada sleep.
func TestReadyzMemoSatuPingPerJendela(t *testing.T) {
	const ttl = 5 * time.Second
	jam := newJamTiruan()
	pinger := &pingerTiruan{}
	app := newHealthApp(pinger, handlers.HealthOptions{MemoTTL: ttl, Now: jam.Now})

	for i := 1; i <= 20; i++ {
		status, body := mustGet(t, app, "/api/v1/readyz")
		if status != http.StatusOK || body != `{"data":{"status":"ready"}}` {
			t.Fatalf("request ke-%d: status = %d, body = %s", i, status, body)
		}
	}
	if n := pinger.calls.Load(); n != 1 {
		t.Fatalf("20 request dalam jendela memo: Pinger dipanggil %d kali, mau 1", n)
	}

	// Tepat sebelum jendela habis: masih dijawab memo.
	jam.Maju(ttl - time.Nanosecond)
	mustGet(t, app, "/api/v1/readyz")
	if n := pinger.calls.Load(); n != 1 {
		t.Fatalf("sebelum jendela habis: Pinger dipanggil %d kali, mau 1", n)
	}

	// Jendela habis: satu ping baru, lalu memo lagi.
	jam.Maju(time.Nanosecond)
	for i := 0; i < 5; i++ {
		mustGet(t, app, "/api/v1/readyz")
	}
	if n := pinger.calls.Load(); n != 2 {
		t.Fatalf("setelah jendela lewat: Pinger dipanggil %d kali, mau 2", n)
	}
}

// Jam yang MUNDUR tidak boleh membekukan memo. time.Now() punya pembacaan
// monotonic, tetapi sumber waktu suntikan (HealthOptions.Now) belum tentu:
// tanpa penjaga, selisih negatif selalu "lebih kecil dari TTL" dan status lama
// terus disajikan sampai jam menyusul lagi (di sini: satu jam).
func TestReadyzJamMundurTidakMembekukanMemo(t *testing.T) {
	const ttl = 5 * time.Second
	jam := newJamTiruan()
	var gagal atomic.Bool
	pinger := &pingerTiruan{fn: func(context.Context) error {
		if gagal.Load() {
			return errors.New("koneksi terputus")
		}
		return nil
	}}
	app := newHealthApp(pinger, handlers.HealthOptions{MemoTTL: ttl, Now: jam.Now})

	if status, body := mustGet(t, app, "/api/v1/readyz"); status != http.StatusOK {
		t.Fatalf("awal: status = %d, mau 200; body=%s", status, body)
	}

	// Database jatuh, lalu jam mundur satu jam.
	gagal.Store(true)
	jam.Maju(-1 * time.Hour)

	status, body := mustGet(t, app, "/api/v1/readyz")
	if n := pinger.calls.Load(); n != 2 {
		t.Fatalf("setelah jam mundur: Pinger dipanggil %d kali, mau 2 (ping ulang)", n)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("setelah jam mundur: status = %d, mau 503 (bukan memo lama); body=%s", status, body)
	}

	// Memo yang baru dicap dengan jam yang sudah mundur tetap berlaku normal.
	mustGet(t, app, "/api/v1/readyz")
	if n := pinger.calls.Load(); n != 2 {
		t.Fatalf("dalam jendela baru: Pinger dipanggil %d kali, mau 2", n)
	}
	jam.Maju(ttl)
	mustGet(t, app, "/api/v1/readyz")
	if n := pinger.calls.Load(); n != 3 {
		t.Fatalf("setelah jendela baru lewat: Pinger dipanggil %d kali, mau 3", n)
	}
}

// Kegagalan ikut di-memo: database yang sedang bermasalah tidak boleh dibanjiri
// ping. Setelah jendela lewat dan database pulih, status kembali siap.
func TestReadyzMemoMenyimpanKegagalan(t *testing.T) {
	const ttl = 5 * time.Second
	jam := newJamTiruan()
	var gagal atomic.Bool
	gagal.Store(true)
	pinger := &pingerTiruan{fn: func(context.Context) error {
		if gagal.Load() {
			return errors.New("dial tcp 10.0.0.5:5432: connection refused")
		}
		return nil
	}}
	app := newHealthApp(pinger, handlers.HealthOptions{MemoTTL: ttl, Now: jam.Now})

	for i := 1; i <= 10; i++ {
		if status, body := mustGet(t, app, "/api/v1/readyz"); status != http.StatusServiceUnavailable {
			t.Fatalf("request ke-%d: status = %d, mau 503; body=%s", i, status, body)
		}
	}
	if n := pinger.calls.Load(); n != 1 {
		t.Fatalf("10 request saat gagal: Pinger dipanggil %d kali, mau 1", n)
	}

	// Database pulih, tetapi memo belum kedaluwarsa: masih 503 tanpa ping baru.
	gagal.Store(false)
	if status, _ := mustGet(t, app, "/api/v1/readyz"); status != http.StatusServiceUnavailable {
		t.Fatalf("memo kegagalan masih berlaku: status = %d, mau 503", status)
	}

	jam.Maju(ttl)
	if status, body := mustGet(t, app, "/api/v1/readyz"); status != http.StatusOK {
		t.Fatalf("setelah jendela lewat dan pulih: status = %d, mau 200; body=%s", status, body)
	}
	if n := pinger.calls.Load(); n != 2 {
		t.Fatalf("Pinger dipanggil %d kali, mau 2", n)
	}
}

// 50 request bersamaan saat memo kosong/kedaluwarsa → tetap satu ping. Pinger
// ditahan sampai semua request sempat masuk, supaya yang diuji benar-benar
// jalur berebut, bukan request yang kebetulan datang setelah memo terisi.
func TestReadyzKonkurenSatuPing(t *testing.T) {
	const n = 50
	mulai := make(chan struct{}, n)
	lepas := make(chan struct{})
	pinger := &pingerTiruan{fn: func(context.Context) error {
		mulai <- struct{}{}
		<-lepas
		return nil
	}}
	app := newHealthApp(pinger, handlers.HealthOptions{PingTimeout: 10 * time.Second})

	statuses := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _, errs[i] = getStatus(app, "/api/v1/readyz")
		}()
	}

	select {
	case <-mulai:
	case <-time.After(3 * time.Second):
		t.Fatal("Pinger tidak pernah dipanggil")
	}
	// Beri waktu request lain menumpuk di belakang ping yang sedang berjalan.
	time.Sleep(100 * time.Millisecond)
	close(lepas)
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if statuses[i] != http.StatusOK {
			t.Errorf("request %d: status = %d, mau 200", i, statuses[i])
		}
	}
	if got := pinger.calls.Load(); got != 1 {
		t.Fatalf("%d request konkuren: Pinger dipanggil %d kali, mau 1", n, got)
	}
}

// Pinger yang lebih lambat dari batas waktu → 503 dalam waktu wajar. Batas
// waktu disuntik 50 ms agar test tidak menunggu 2 detik.
func TestReadyzPingerLambat503(t *testing.T) {
	const timeout = 50 * time.Millisecond
	const batasWajar = 2 * time.Second

	t.Run("pinger menghormati context", func(t *testing.T) {
		pinger := &pingerTiruan{fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		app := newHealthApp(pinger, handlers.HealthOptions{PingTimeout: timeout})

		start := time.Now()
		status, body := mustGet(t, app, "/api/v1/readyz")
		if elapsed := time.Since(start); elapsed > batasWajar {
			t.Errorf("respons butuh %s, mau di bawah %s", elapsed, batasWajar)
		}
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, mau 503; body=%s", status, body)
		}
		if strings.Contains(body, "deadline") {
			t.Errorf("body membocorkan galat timeout: %s", body)
		}
	})

	// Driver yang MENGABAIKAN pembatalan context tidak boleh menggantung
	// request, dan tidak boleh membuat ping/goroutine menumpuk.
	t.Run("pinger mengabaikan context", func(t *testing.T) {
		const ttl = 5 * time.Second
		jam := newJamTiruan()
		lepas := make(chan struct{})
		var macet atomic.Bool
		macet.Store(true)
		pinger := &pingerTiruan{fn: func(context.Context) error {
			if macet.Load() {
				<-lepas
			}
			return nil
		}}
		app := newHealthApp(pinger, handlers.HealthOptions{PingTimeout: timeout, MemoTTL: ttl, Now: jam.Now})

		start := time.Now()
		status, body := mustGet(t, app, "/api/v1/readyz")
		if elapsed := time.Since(start); elapsed > batasWajar {
			t.Errorf("respons butuh %s, mau di bawah %s", elapsed, batasWajar)
		}
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, mau 503; body=%s", status, body)
		}

		// Memo kedaluwarsa sementara ping pertama masih tersangkut: tetap 503
		// dan TIDAK ada ping kedua.
		jam.Maju(ttl)
		if status, _ := mustGet(t, app, "/api/v1/readyz"); status != http.StatusServiceUnavailable {
			t.Fatalf("selama ping lama tersangkut: status = %d, mau 503", status)
		}
		if n := pinger.calls.Load(); n != 1 {
			t.Fatalf("ping menumpuk: Pinger dipanggil %d kali, mau 1", n)
		}

		// Ping lama akhirnya kembali; pemeriksaan berikutnya berjalan normal.
		macet.Store(false)
		close(lepas)
		jam.Maju(ttl)
		tenggat := time.Now().Add(3 * time.Second)
		for {
			status, body := mustGet(t, app, "/api/v1/readyz")
			if status == http.StatusOK {
				break
			}
			if time.Now().After(tenggat) {
				t.Fatalf("tidak pulih setelah ping lama selesai: status = %d; body=%s", status, body)
			}
			// Goroutine ping lama belum sempat mengirim hasilnya; coba lagi.
			time.Sleep(5 * time.Millisecond)
			jam.Maju(ttl)
		}
		if n := pinger.calls.Load(); n != 2 {
			t.Fatalf("setelah pulih: Pinger dipanggil %d kali, mau 2", n)
		}

		// Pulihnya harus BERTAHAN. Penanda ping tersangkut wajib dibersihkan
		// begitu ping lama kembali; kalau tidak, pemeriksaan berikutnya masih
		// mengira ada ping yang berjalan dan readiness jadi 503 selamanya
		// setelah satu kali timeout. Dua jendela diuji, bukan satu.
		for putaran := 1; putaran <= 2; putaran++ {
			jam.Maju(ttl)
			if status, body := mustGet(t, app, "/api/v1/readyz"); status != http.StatusOK {
				t.Fatalf("jendela ke-%d setelah pulih: status = %d, mau 200; body=%s", putaran, status, body)
			}
			if n, mau := pinger.calls.Load(), int32(2+putaran); n != mau {
				t.Fatalf("jendela ke-%d setelah pulih: Pinger dipanggil %d kali, mau %d", putaran, n, mau)
			}
		}
	})
}

// Tanpa pinger, setiap request readiness dijawab 503 tanpa pernah sampai ke
// ping, jadi penyebabnya harus tercatat SEKALI saat handler dirakit — dan
// tidak sekali per request (endpoint publik: log per request = banjir log).
func TestReadyzTanpaPingerDilogSekaliSaatPerakitan(t *testing.T) {
	const pesan = "pinger database tidak tersedia"

	var buf bytes.Buffer
	lama := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = lama })

	app := newHealthApp(nil, handlers.HealthOptions{})
	if n := strings.Count(buf.String(), pesan); n != 1 {
		t.Fatalf("saat perakitan: pesan %q muncul %d kali di log, mau 1.\nlog:\n%s", pesan, n, buf.String())
	}

	for i := 1; i <= 5; i++ {
		if status, body := mustGet(t, app, "/api/v1/readyz"); status != http.StatusServiceUnavailable {
			t.Fatalf("request ke-%d: status = %d, mau 503; body=%s", i, status, body)
		}
	}
	if n := strings.Count(buf.String(), pesan); n != 1 {
		t.Errorf("setelah 5 request: pesan %q muncul %d kali di log, mau tetap 1.\nlog:\n%s", pesan, n, buf.String())
	}

	// Dengan pinger yang ada, tidak ada peringatan sama sekali.
	buf.Reset()
	newHealthApp(&pingerTiruan{}, handlers.HealthOptions{})
	if strings.Contains(buf.String(), pesan) {
		t.Errorf("pinger tersedia tetapi peringatan tetap di-log:\n%s", buf.String())
	}
}

// Tanpa Pinger (router dirakit tanpa database) atau dengan *sql.DB nil yang
// terbungkus antarmuka: 503 generik, bukan panic.
func TestReadyzTanpaPingerTidakPanic(t *testing.T) {
	var dbNil *sql.DB

	cases := []struct {
		name   string
		pinger handlers.Pinger
	}{
		{"pinger nil", nil},
		{"*sql.DB nil terbungkus antarmuka", dbNil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newHealthApp(tc.pinger, handlers.HealthOptions{})

			status, body := mustGet(t, app, "/api/v1/readyz")
			if status != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, mau 503; body=%s", status, body)
			}
			errObj, _ := decode(t, []byte(body))["error"].(map[string]any)
			if errObj["message"] != "layanan sedang terganggu" {
				t.Errorf("error.message = %v, mau pesan generik", errObj["message"])
			}
			if strings.Contains(body, "panic") || strings.Contains(body, "nil pointer") {
				t.Errorf("body membocorkan detail internal: %s", body)
			}

			// Liveness tidak terpengaruh.
			if status, _ := mustGet(t, app, "/api/v1/healthz"); status != http.StatusOK {
				t.Errorf("healthz: status = %d, mau 200", status)
			}
		})
	}
}
