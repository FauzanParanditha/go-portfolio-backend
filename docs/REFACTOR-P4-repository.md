# Desain Refactor P4 — Kembalikan Konsistensi Arsitektur Berlapis (Repository)

> Status: SELESAI (terimplementasi). Tujuan: memindahkan seluruh akses DB dari handler
> yang masih memakai `*gorm.DB` mentah ke **repository interface**, sesuai invarian
> arsitektur di `CLAUDE.md` (handler → repository → model). Kontrak API TIDAK berubah:
> envelope, status code, validasi, query parameterized tetap sama. Tidak ada migration.

Handler yang melanggar dan menjadi target refactor:

| Handler | Konstruktor sekarang | Pelanggaran |
|---|---|---|
| `auth_handler.go` | `NewAuthHandler(db *gorm.DB, ...)` | Konstruktor terima `*gorm.DB` (walau di dalam sudah bikin `UserRepository`). Login sudah pakai repo; Refresh/Logout tak sentuh DB. |
| `me_handler.go` | `NewMeHandler(db *gorm.DB)` | Query `First(&user, "id = ?", ...)` langsung di handler (fallback). |
| `admin_dashboard_handler.go` | `NewAdminDashboardHandler(db *gorm.DB)` | 9 query `Count()` lintas 3 tabel di handler. |
| `admin_experience_handler.go` | `NewAdminExperienceHandler(db *gorm.DB)` | CRUD penuh + transaksi `Begin()` + relasi highlights/tags di handler. |
| `admin_project_handler.go` | `NewAdminProjectHandler(db *gorm.DB)` | CRUD penuh + transaksi `Begin()` + relasi screenshots/features/tags di handler. |

Acuan pola bersih: `admin_tag_handler.go`, `admin_contact_handler.go`, `project_handler.go`,
`experience_handler.go` (+ `internal/repository/*_repository.go`).

Catatan: `admin_contact_handler.go` masih menyimpan field `db *gorm.DB` yang tak dipakai —
sekalian dibersihkan (lihat Tahap 5, opsional).

---

## 1. Inventaris query per handler

### 1.1 `auth_handler.go`
- **Login**: `userRepo.FindByEmail(ctx, email)` — SUDAH lewat repo. Handler lalu
  `bcrypt.CompareHashAndPassword` + `issueToken`. Tidak ada query mentah.
- **Refresh**: tidak menyentuh DB (baca klaim dari `c.Locals`, terbitkan token baru).
- **Logout**: tidak menyentuh DB (hanya denylist + clear cookie).
- **Pelanggaran nyata**: hanya di **konstruktor** yang menerima `*gorm.DB` lalu memanggil
  `repository.NewUserRepository(db)` di dalam. Idealnya konstruktor menerima
  `repository.UserRepository` (di-inject dari router), konsisten dengan handler lain.

### 1.2 `me_handler.go`
- **Me**: jalur cepat baca `c.Locals` (tanpa DB). Jalur fallback (bila `name`/`email`
  kosong): `db.WithContext(ctx).First(&user, "id = ?", userID)`.
- **Operasi DB**: 1× `SELECT users WHERE id = ?` (tanpa preload, tanpa tx).
- **Kebutuhan repo**: `UserRepository.FindByID(ctx, id)`.

### 1.3 `admin_dashboard_handler.go`
- **Overview**: 9 agregasi `Count()`:
  - `projects`: total; `is_featured = true`; `created_at >= since`.
  - `experiences`: total; `is_current = true`; `created_at >= since`.
  - `contact_messages`: total; `is_read = false`; `created_at >= since`.
- Tidak ada tx, tidak ada preload. `since = now - 30 hari`.
- **Catatan**: handler ini juga membangun error JSON manual (`{"message": ...}`), tidak
  memakai `fiber.NewError` seperti handler lain — sekalian diseragamkan saat refactor
  (perilaku status 500 tetap sama, hanya bentuk error envelope jadi konsisten). Ini
  perubahan kecil pada bentuk body error 500; konfirmasi ke pemilik bila FE bergantung
  pada `message`. Envelope sukses `{"data": resp}` TIDAK berubah.

### 1.4 `admin_experience_handler.go`
- **List**: preload `Highlights` (order `sort_order ASC`) + `Tags`. Filter `q` ILIKE atas
  `title`/`company`. `Count` lalu `Order sort_order ASC, start_date DESC` + limit/offset.
- **GetByID**: preload `Highlights`+`Tags`, `First WHERE id = ?`. `ErrRecordNotFound`→404.
- **Create**: `tx.Begin()` → resolve tags by `id IN ?` → `Create(exp)` → insert
  `ExperienceHighlight` (dari `[]string`, index=sort_order) → `Commit` → reload preload.
- **Update**: `tx.Begin()` → `First id` (404) → set scalar → `Save` → tags
  `Association.Replace`/`Clear` → hapus highlights lama + insert baru → `Commit` → reload.
- **Delete**: `Delete(Experience) WHERE id = ?` (tanpa tx).
- Parsing tanggal (`helpers.ParseDateStr`) & `parseTagIDs` = validasi request → **tetap di
  handler**, hasilnya (scalar `models.Experience` + `[]uuid.UUID` + `[]string highlights`)
  dioper ke repo.

### 1.5 `admin_project_handler.go`
- **List**: preload `Features`(order)+`Tags`+`Screenshots`(order). Filter `q` ILIKE atas
  `title`/`short_desc`/`long_desc`/`category`; filter `featured`. `Count` lalu
  `Order sort_order ASC, created_at DESC` + limit/offset.
- **GetByID**: preload 3 relasi, `First WHERE id = ?`. `ErrRecordNotFound`→404.
- **Create**: `tx.Begin()` → resolve tags `id IN ?` → `Create(project)` → insert
  `ProjectFeature[]` + `ProjectScreenshot[]` (dari `[]string`, index=sort_order) → `Commit`
  → reload preload. `TechnicalDetails` (map→`datatypes.JSON`) di-marshal di handler.
- **Update**: `tx.Begin()` → `First id` (404) → set scalar (JSON hanya bila non-nil) →
  `Save` → tags `Replace`/`Clear` → delete+reinsert features → delete+reinsert screenshots
  → `Commit` → reload.
- **Delete**: `Delete(Project) WHERE id = ?` (tanpa tx).
- Marshal `TechnicalDetails` & `parseTagIDs` = pemrosesan request → **tetap di handler**.

---

## 2. Kontrak method interface baru

Gaya mengikuti repo yang ada: `ctx context.Context` argumen pertama, kembalikan
`*model`/`[]model` + `error`, `gorm.ErrRecordNotFound` **diteruskan apa adanya** agar
handler yang memetakan ke 404 (bandingkan `err == gorm.ErrRecordNotFound`). Semua `Where`
tetap parameterized. Transaksi dikelola di dalam repo (lihat §3).

### 2.1 `UserRepository` (perluas `user_repository.go` yang sudah ada)

```go
type UserRepository interface {
    FindByEmail(ctx context.Context, email string) (*models.User, error) // sudah ada
    FindByID(ctx context.Context, id string) (*models.User, error)       // BARU
}
```

- `FindByID`: `SELECT users WHERE id = ?` (tanpa preload). Teruskan
  `gorm.ErrRecordNotFound`. Dipakai `me_handler` (fallback). Refresh TIDAK butuh DB — biarkan.

### 2.2 `ProjectRepository` (perluas `project_repository.go`)

Param list admin dipisah dari public agar sinyal filter jelas:

```go
type ProjectAdminListParams struct {
    Query        string
    FeaturedOnly bool
    Page         int
    Limit        int
}

// Agregat tulis: scalar sudah dipetakan handler, relasi dikirim sebagai data mentah.
type ProjectWriteInput struct {
    Project     models.Project // scalar saja (termasuk TechnicalDetails datatypes.JSON)
    TagIDs      []uuid.UUID
    Features    []string       // index = sort_order; string kosong di-skip di repo
    Screenshots []string       // index = sort_order; string kosong di-skip di repo
}

type ProjectRepository interface {
    ListPublic(ctx context.Context, params ProjectListParams) ([]models.Project, int64, error) // sudah ada
    GetBySlug(ctx context.Context, slug string) (*models.Project, error)                        // sudah ada

    ListAdmin(ctx context.Context, params ProjectAdminListParams) ([]models.Project, int64, error) // BARU
    GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Project, error)                        // BARU
    Create(ctx context.Context, in ProjectWriteInput) (*models.Project, error)                      // BARU (tx)
    Update(ctx context.Context, id uuid.UUID, in ProjectWriteInput) (*models.Project, error)        // BARU (tx)
    Delete(ctx context.Context, id uuid.UUID) error                                                 // BARU
}
```

Semantik & error:
- `ListAdmin`: preload `Features`(order `sort_order ASC`)+`Tags`+`Screenshots`(order),
  filter `q` (ILIKE 4 kolom, gaya sama seperti handler saat ini), filter `featured`,
  `Count` + `Order sort_order ASC, created_at DESC` + limit/offset. Kembalikan
  `([]Project, total, err)`. (Boleh reuse `baseQuery()` yang sudah ada.)
- `GetByIDAdmin`: preload 3 relasi, `First WHERE id = ?`. Teruskan `ErrRecordNotFound`.
- `Create`: buka transaksi internal → resolve tags (`id IN ?`) → `Create(project)` →
  bangun & insert `ProjectFeature[]`/`ProjectScreenshot[]` (skip string kosong,
  `SortOrder=i`) → commit → **reload dengan preload** dan kembalikan `*Project`. Error DB
  apa pun dikembalikan (handler map ke 500).
- `Update`: transaksi internal → `First id` (teruskan `ErrRecordNotFound`) → overwrite
  scalar dari `in.Project` (TechnicalDetails hanya ditimpa bila non-nil — logika keputusan
  itu tetap di handler; handler mengirim `Project.TechnicalDetails` yang sudah final) →
  `Save` → tags `Replace` bila ada, `Clear` bila kosong → delete+reinsert features →
  delete+reinsert screenshots → commit → reload → kembalikan `*Project`.
- `Delete`: `Delete(Project{}) WHERE id = ?`. (Perilaku sekarang: sukses walau 0 baris —
  dipertahankan; handler tetap balas 204.)

> Catatan `TechnicalDetails` pada Update: handler saat ini hanya menimpa bila body punya
> `technicalDetails`. Agar repo tak perlu tahu "apakah field dikirim", **handler** memuat
> nilai lama? Tidak perlu — cukup: handler set `in.Project.TechnicalDetails` hanya bila ada
> input, dan repo saat Update meng-`Save` seluruh scalar. Untuk mempertahankan perilaku
> "jangan hapus JSON lama saat body tak mengirim", repo harus **tidak menimpa** kolom itu
> bila `in.Project.TechnicalDetails == nil`. Rekomendasi: repo cek `if in.Project.TechnicalDetails != nil { existing.TechnicalDetails = in.Project.TechnicalDetails }`
> (sama persis dengan cabang `if technicalDetails != nil` di handler sekarang). Dokumentasikan
> ini di komentar method agar tidak regres.

### 2.3 `ExperienceRepository` (perluas `experience_repository.go`)

```go
type ExperienceAdminListParams struct {
    Query string
    Page  int
    Limit int
}

type ExperienceWriteInput struct {
    Experience models.Experience // scalar (Title, Company, Location, StartDate, EndDate, IsCurrent, Description, SortOrder)
    TagIDs     []uuid.UUID
    Highlights []string          // index = sort_order; kosong di-skip
}

type ExperienceRepository interface {
    ListPublic(ctx context.Context) ([]models.Experience, error) // sudah ada

    ListAdmin(ctx context.Context, params ExperienceAdminListParams) ([]models.Experience, int64, error) // BARU
    GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Experience, error)                          // BARU
    Create(ctx context.Context, in ExperienceWriteInput) (*models.Experience, error)                     // BARU (tx)
    Update(ctx context.Context, id uuid.UUID, in ExperienceWriteInput) (*models.Experience, error)       // BARU (tx)
    Delete(ctx context.Context, id uuid.UUID) error                                                      // BARU
}
```

Semantik identik pola Project, relasi = `Highlights`(order `sort_order ASC`)+`Tags`; filter
`q` ILIKE atas `title`/`company`; order `sort_order ASC, start_date DESC`. Parsing tanggal
`StartDate`/`EndDate` (string→`time.Time`) tetap di handler; handler mengisi
`in.Experience.StartDate/EndDate` yang sudah jadi `time.Time`. `ErrRecordNotFound`
diteruskan pada `GetByIDAdmin`/`Update`.

### 2.4 Dashboard — **repository baru** `DashboardRepository`

Rekomendasi: **buat `dashboard_repository.go` baru** (bukan menambah `Count*` ke tiga repo
berbeda) supaya agregasi lintas-tabel terkumpul di satu tempat dan handler tinggal
memetakan ke response. Repo mengembalikan struct count netral (tak tergantung DTO
response), handler menyusun `models.DashboardOverviewResponse` + info `System`.

```go
// Struct hasil agregasi, netral terhadap bentuk response.
type DashboardCounts struct {
    ProjectsTotal      int64
    ProjectsFeatured   int64
    ProjectsRecent     int64
    ExperiencesTotal   int64
    ExperiencesCurrent int64
    ExperiencesRecent  int64
    ContactTotal       int64
    ContactUnread      int64
    ContactRecent      int64
}

type DashboardRepository interface {
    // Overview menghitung seluruh agregat sejak `since` (batas "recent").
    Overview(ctx context.Context, since time.Time) (DashboardCounts, error)
}
```

- Handler menghitung `since = now - 30d`, panggil `Overview`, lalu isi
  `DashboardOverviewResponse` (termasuk `System.ServerTime`, `System.RecentDays`) dan balas
  `{"data": resp}`. Bila error, `fiber.NewError(500, ...)` (seragam, menggantikan
  `{"message": ...}` manual).
- Alternatif yang ditolak: menyebar `CountFeatured`/`CountRecent` ke tiap resource repo —
  membuat orkestrasi dashboard bocor kembali ke handler dan menambah banyak method sempit.

---

## 3. Strategi transaksi (tanpa membocorkan `*gorm.DB` ke handler)

Ganti pola manual `tx := db.Begin(); defer recover; tx.Rollback()/Commit()` di handler
dengan **`gorm.DB.Transaction(fn)` di dalam method repository**. Handler tidak pernah
melihat `tx`/`*gorm.DB`.

Pola untuk `Create`/`Update` (Project & Experience):

```go
func (r *projectRepository) Create(ctx context.Context, in ProjectWriteInput) (*models.Project, error) {
    project := in.Project
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if len(in.TagIDs) > 0 {
            var tags []models.Tag
            if err := tx.Where("id IN ?", in.TagIDs).Find(&tags).Error; err != nil {
                return err // otomatis rollback
            }
            project.Tags = tags
        }
        if err := tx.Create(&project).Error; err != nil {
            return err
        }
        // features + screenshots dari in.Features/in.Screenshots (skip kosong, SortOrder=i)
        // ...
        return nil // otomatis commit
    })
    if err != nil {
        return nil, err
    }
    return r.reloadWithRelations(ctx, project.ID) // preload 3 relasi
}
```

Keuntungan: rollback/commit ditangani `Transaction` (rollback otomatis bila fn balas error
atau panic), tak ada `defer recover` manual, boundary tx murni di repo. `reloadWithRelations`
memakai `r.db` (koneksi normal, di luar tx) — sama seperti perilaku "reload" handler
sekarang. Untuk `Update`, `First id` dilakukan **di dalam** `Transaction` agar
`ErrRecordNotFound` mengalir keluar sebagai error transaksi lalu diteruskan ke handler.

Semua query di dalam tx tetap parameterized (`Where("id IN ?", ...)`,
`Where("project_id = ?", ...)`).

---

## 4. Rencana bertahap (tugas untuk backend-engineer)

Urutan dari paling sederhana → paling kompleks. Setiap tahap harus tetap meng-compile,
`make vet` bersih, dan mempertahankan kontrak API. **`router.go` disentuh hampir tiap
tahap** (fungsi `registerX` berbeda) → bila dikerjakan paralel oleh beberapa agen, `router.go`
jadi titik konflik; rekomendasi **kerjakan serial** (satu owner file) atau lakukan satu
"pass wiring" di akhir. `cmd/api/main.go` **tidak** perlu berubah (semua konstruktor
di-wire di `router.go`; `AppDeps` tetap membawa `DB`).

### Tahap 1 — Me + Auth (paling ringan)  · serial-1
- `internal/repository/user_repository.go`: tambah `FindByID(ctx, id)`.
- `internal/http/handlers/me_handler.go`: `MeHandler{ userRepo repository.UserRepository }`,
  `NewMeHandler(userRepo)`; fallback panggil `userRepo.FindByID`. Perilaku Locals-fast-path
  tetap.
- `internal/http/handlers/auth_handler.go`: ubah konstruktor jadi
  `NewAuthHandler(userRepo repository.UserRepository, cfg, dl)` (hilangkan `*gorm.DB` &
  import `gorm`). Body Login/Refresh/Logout tak berubah.
- `internal/http/router.go`: di `registerAuthRoutes` & `registerAuthMeRoutes`, buat
  `userRepo := repository.NewUserRepository(deps.DB)` lalu inject. (Boleh satu instance
  dibagi.)
- DoD: `go build ./...`, `make vet`, endpoint `/me`, `/auth/*` tak berubah perilaku.

### Tahap 2 — Dashboard (independen, file repo baru)  · bisa paralel dgn Tahap 1 KECUALI router.go
- `internal/repository/dashboard_repository.go` (BARU): `DashboardRepository` +
  `DashboardCounts` + impl `Overview(ctx, since)` (9 `Count`).
- `internal/http/handlers/admin_dashboard_handler.go`:
  `AdminDashboardHandler{ repo repository.DashboardRepository }`,
  `NewAdminDashboardHandler(repo)`; susun `DashboardOverviewResponse`; error pakai
  `fiber.NewError` (seragam).
- `internal/http/router.go`: `registerAdminDasbboardRoute` inject repo baru.
- DoD: response `{"data": {...}}` byte-identik untuk data sama; hanya body error 500 yang
  berubah bentuk (konfirmasi FE bila perlu).

### Tahap 3 — Admin Experience  · serial-2
- `internal/repository/experience_repository.go`: tambah `ExperienceAdminListParams`,
  `ExperienceWriteInput`, dan method `ListAdmin/GetByIDAdmin/Create/Update/Delete` (tx via
  `Transaction`, §3).
- `internal/http/handlers/admin_experience_handler.go`: `db` → `repo repository.ExperienceRepository`.
  Handler tetap: validasi, parse tanggal, `parseTagIDs`, bangun `models.Experience` scalar +
  `ExperienceWriteInput`, panggil repo, map hasil ke `experienceToResponse`; `ErrRecordNotFound`
  → 404. Import `gorm` kemungkinan masih perlu untuk cek `ErrRecordNotFound`.
- `internal/http/router.go`: `registerAdminExperienceRoutes` inject repo.
- DoD: 5 endpoint experiences identik (envelope, 404, 422, urutan, pagination).

### Tahap 4 — Admin Project (paling kompleks)  · serial-3
- `internal/repository/project_repository.go`: tambah `ProjectAdminListParams`,
  `ProjectWriteInput`, method admin + tx (§2.2, §3). Perhatikan aturan `TechnicalDetails`
  non-nil saat Update.
- `internal/http/handlers/admin_project_handler.go`: `db` → `repo repository.ProjectRepository`.
  Handler tetap: validasi, marshal `TechnicalDetails`, `parseTagIDs`, bangun scalar +
  `ProjectWriteInput`, panggil repo, map ke `projectToResponse`; `ErrRecordNotFound` → 404.
  Helper `sendValidationError` & `parseTagIDs` (tak sentuh DB) tetap tinggal di file ini
  (juga dipakai handler experience — jangan pindah/hapus).
- `internal/http/router.go`: `registerAdminProjectRoutes` inject repo.
- DoD: 5 endpoint projects identik; transaksi tetap atomik.

### Tahap 5 (opsional, pembersihan) — hapus `db` mati di `admin_contact_handler.go`
- Buang field `db *gorm.DB` yang tak dipakai + sederhanakan `NewAdminContactHandler` jadi
  hanya terima `repository.ContactMessageRepository`. Sesuaikan `registerAdminContactRoutes`.
  Murni kosmetik; boleh dilewati bila ingin menjaga diff minimal.

### Paralelisasi
- Isi **repo** tiap resource ada di file terpisah (`user_/project_/experience_/dashboard_`)
  → bisa dikerjakan paralel. **Namun** semua tahap menyentuh `router.go` (dan Tahap 1
  menyentuh `auth_handler.go`+`me_handler.go`). Rekomendasi praktis: **serial** urut 1→2→3→4
  (satu engineer), atau paralelkan penulisan repo lalu satu commit wiring `router.go` di
  akhir. Hindari dua agen mengedit `router.go` bersamaan.

---

## 5. Dampak & risiko

- **Wiring**: hanya `internal/http/router.go` berubah (konstruktor handler menerima repo,
  bukan `deps.DB`). `cmd/api/main.go` & `AppDeps` **tidak** berubah — `deps.DB` tetap
  dipakai untuk membuat repo di router. Tidak ada perubahan tanda tangan `NewRouter`.
- **Migration**: TIDAK diperlukan. Skema DB tidak berubah — refactor kode murni.
- **Kontrak API**: harus tetap sama (envelope `{data}`/`{data,meta}`, 400/404/422/500,
  urutan `Order`, pagination clamp `limit>100→100`). Satu-satunya perubahan bentuk yang
  disengaja: body **error 500 dashboard** dari `{"message": ...}` → envelope error standar
  (`fiber.NewError`). Tandai untuk dikonfirmasi ke pemilik/FE; sukses tidak berubah.
- **Keamanan**: query tetap parameterized (`Where(... ?, val)`), tidak ada string-concat.
  Tidak ada perubahan pada rantai `AuthJWT`+`RequireRole("admin")` di grup admin. `User.Password`
  tetap `json:"-"`. Tidak ada perubahan yang menyentuh auth-boundary → audit keamanan cukup
  ringan (spot-check bahwa tak ada regresi injection & role guard). Serahkan ke
  `security-auditor` hanya bila diinginkan; risiko rendah.

### Peluang test DB-free (untuk qa-engineer setelah refactor)
Karena handler kini bergantung pada **interface**, bisa dibuat fake repo (implementasi
in-memory) dan diuji lewat `app.Test(...)` + JWT bertanda tangan, tanpa Postgres:
- **me_handler**: fake `UserRepository.FindByID` → assert 200 + shape `MeResponse`; jalur
  Locals-fast-path (tanpa panggil repo) juga.
- **auth_handler.Login**: fake `FindByEmail` mengembalikan user dengan hash bcrypt →
  assert 200/`data.token`; user tak ada → 401; password salah → 401; body invalid → 400;
  validasi gagal → 422.
- **admin_project / admin_experience**: fake repo mengembalikan data kanonik → assert
  envelope+`meta` pagination; `GetByID` dengan `gorm.ErrRecordNotFound` → 404; body invalid
  → 400/422; `Create/Update` sukses → 201/200 shape benar. Ini menutup jalur yang selama
  ini tak bisa diuji karena butuh DB + tx.
- **admin_dashboard**: fake `DashboardRepository.Overview` → assert seluruh angka termuat &
  `system.recentDays == 30`.
- Pertahankan pola test DB-free yang sudah ada di
  `internal/http/middleware/require_role_test.go`.
```
