// Command import memasukkan data proyek dari berkas JSON langsung ke database,
// tanpa lewat panel admin.
//
// Dibuat agar isi portfolio bisa disiapkan di luar aplikasi — mis. digenerate
// oleh agen lain — lalu dimuat sekali jalan.
//
// Dua keputusan yang membuat berkasnya enak ditulis manusia/agen:
//
//  1. Tag ditulis dengan NAMA + TIPE, bukan UUID. Penulis berkas tidak mungkin
//     tahu UUID; perintah ini yang mencari tag berdasarkan nama (case-insensitive)
//     dan membuatnya bila belum ada.
//  2. Upsert berdasarkan `slug`. Menjalankan perintah yang sama dua kali tidak
//     menghasilkan duplikat — proyek dengan slug yang sudah ada akan diperbarui.
//     Jadi berkasnya bisa diperlakukan sebagai sumber kebenaran dan dimuat ulang
//     kapan saja.
//
// Pemakaian:
//
//	make import file=docs/contoh-import-proyek.json          # muat
//	make import file=... args=-dry                           # pratinjau saja
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/db"
	"github.com/FauzanParanditha/portfolio-backend/internal/logger"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// tagRef adalah rujukan tag dalam berkas impor: nama + tipe, bukan UUID.
//
// `type` dipakai mengelompokkan keahlian di section About beranda, jadi sebaiknya
// konsisten dengan tipe yang sudah ada (backend, frontend, database, devops,
// framework, language, tools, cloud).
type tagRef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// projectInput adalah satu proyek di berkas impor.
//
// Nama field-nya sengaja SAMA dengan response API publik, sehingga hasil
// `GET /api/v1/projects/:slug` bisa disunting lalu dimasukkan kembali.
type projectInput struct {
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	ShortDesc     string `json:"shortDesc"`
	LongDesc      string `json:"longDescription"`
	CoverImageURL string `json:"coverImageUrl"`

	Category string `json:"category"`
	Timeline string `json:"timeline"`
	Role     string `json:"role"`

	Challenge string `json:"challenge"`
	Solution  string `json:"solution"`

	Results          []string       `json:"results"`
	TechnicalDetails map[string]any `json:"technicalDetails"`

	DemoURL *string `json:"demoUrl"`
	RepoURL *string `json:"repoUrl"`

	Screenshots []string `json:"screenshots"`
	Features    []string `json:"features"`
	Tags        []tagRef `json:"tags"`

	IsFeatured bool `json:"isFeatured"`
	SortOrder  int  `json:"sortOrder"`
}

// validate memeriksa field yang WAJIB ada. Sengaja dijalankan untuk SEMUA entri
// sebelum satu pun ditulis, supaya berkas yang cacat tidak masuk separuh.
func (p projectInput) validate(index int) error {
	var missing []string
	if strings.TrimSpace(p.Title) == "" {
		missing = append(missing, "title")
	}
	if strings.TrimSpace(p.Slug) == "" {
		missing = append(missing, "slug")
	}
	if strings.TrimSpace(p.ShortDesc) == "" {
		missing = append(missing, "shortDesc")
	}
	if len(missing) > 0 {
		return fmt.Errorf("proyek #%d (%q): field wajib kosong: %s",
			index+1, p.Title, strings.Join(missing, ", "))
	}

	// Impor tidak lewat DTO handler, jadi aturan URL yang sama diterapkan di
	// sini secara eksplisit. Tanpa ini, berkas impor menjadi jalan pintas untuk
	// menyimpan `javascript:` ke demoUrl/repoUrl yang dirender sebagai `href`.
	var invalid []string
	for _, f := range []struct {
		name string
		val  *string
	}{{"demoUrl", p.DemoURL}, {"repoUrl", p.RepoURL}} {
		if f.val != nil && *f.val != "" && !validation.IsHTTPURL(*f.val) {
			invalid = append(invalid, f.name)
		}
	}
	// coverImageUrl boleh kosong di impor (lihat contoh berkas), tapi bila diisi
	// harus sumber gambar yang aman.
	if p.CoverImageURL != "" && !validation.IsImageURL(p.CoverImageURL) {
		invalid = append(invalid, "coverImageUrl")
	}
	// Screenshot "" dilewati, sama seperti DTO handler (dive,omitempty) dan
	// repository yang tidak menyimpan string kosong.
	for i, s := range p.Screenshots {
		if s != "" && !validation.IsImageURL(s) {
			invalid = append(invalid, fmt.Sprintf("screenshots[%d]", i))
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("proyek #%d (%q): URL tidak valid (hanya http/https, atau path \"/...\" untuk gambar): %s",
			index+1, p.Title, strings.Join(invalid, ", "))
	}
	return nil
}

func main() {
	file := flag.String("file", "", "path berkas JSON berisi data proyek (wajib)")
	dry := flag.Bool("dry", false, "hanya tampilkan rencana, tidak menulis ke database")
	flag.Parse()

	_ = godotenv.Load()
	cfg := config.Load()
	logger.Init(cfg.AppEnv)

	if *file == "" {
		log.Fatal().Msg("wajib menyertakan -file, mis: make import file=data/proyek.json")
	}

	projects, err := readProjects(*file)
	if err != nil {
		log.Fatal().Err(err).Msg("gagal membaca berkas impor")
	}
	if len(projects) == 0 {
		log.Fatal().Msg("berkas tidak memuat satu pun proyek")
	}

	// Validasi seluruh berkas lebih dulu: lebih baik gagal tanpa menulis apa pun
	// daripada memasukkan separuh data lalu berhenti di tengah.
	for i, p := range projects {
		if err := p.validate(i); err != nil {
			log.Fatal().Err(err).Msg("berkas impor tidak valid")
		}
	}
	// Slug ganda dalam satu berkas: karena upsert memakai slug sebagai kunci,
	// entri terakhir akan menimpa yang sebelumnya tanpa jejak. Ditolak supaya
	// tidak ada data yang hilang diam-diam.
	urutanSlug := make(map[string]int, len(projects))
	for i, p := range projects {
		slug := strings.ToLower(strings.TrimSpace(p.Slug))
		if sebelumnya, ada := urutanSlug[slug]; ada {
			log.Fatal().Msgf(
				"slug %q dipakai dua kali (proyek #%d dan #%d): upsert akan saling menimpa",
				p.Slug, sebelumnya+1, i+1)
		}
		urutanSlug[slug] = i
	}

	log.Info().Int("jumlah", len(projects)).Str("berkas", *file).Msg("berkas impor valid")

	if *dry {
		for i, p := range projects {
			log.Info().
				Int("no", i+1).
				Str("slug", p.Slug).
				Str("title", p.Title).
				Int("tags", len(p.Tags)).
				Int("features", len(p.Features)).
				Int("screenshots", len(p.Screenshots)).
				Bool("featured", p.IsFeatured).
				Msg("[dry-run] akan di-upsert")
		}
		log.Info().Msg("dry-run selesai; tidak ada yang ditulis")
		return
	}

	gormDB := db.New(cfg)
	projectRepo := repository.NewProjectRepository(gormDB)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var created, updated int
	for i, p := range projects {
		tagIDs, err := resolveTags(ctx, gormDB, p.Tags)
		if err != nil {
			log.Fatal().Err(err).Str("slug", p.Slug).Msg("gagal menyiapkan tag")
		}

		in := repository.ProjectWriteInput{
			Project:     toModel(p),
			TagIDs:      tagIDs,
			Features:    p.Features,
			Screenshots: p.Screenshots,
		}

		existing, err := projectRepo.GetBySlug(ctx, p.Slug)
		switch {
		case err == nil && existing != nil:
			if _, err := projectRepo.Update(ctx, existing.ID, in); err != nil {
				log.Fatal().Err(err).Str("slug", p.Slug).Msg("gagal memperbarui proyek")
			}
			updated++
			log.Info().Int("no", i+1).Str("slug", p.Slug).Msg("proyek diperbarui")

		case errors.Is(err, gorm.ErrRecordNotFound):
			if _, err := projectRepo.Create(ctx, in); err != nil {
				log.Fatal().Err(err).Str("slug", p.Slug).Msg("gagal membuat proyek")
			}
			created++
			log.Info().Int("no", i+1).Str("slug", p.Slug).Msg("proyek dibuat")

		default:
			log.Fatal().Err(err).Str("slug", p.Slug).Msg("gagal memeriksa proyek yang sudah ada")
		}
	}

	log.Info().Int("dibuat", created).Int("diperbarui", updated).Msg("impor selesai")
}

// readProjects membaca berkas dan menerima dua bentuk JSON: objek berkunci
// "projects", atau array telanjang.
//
// Field di luar skema DITOLAK, tidak diabaikan. Alasannya konkret: berkas
// keluaran agen pernah memuat `"visibility": "internal"` — penanda yang, kalau
// diabaikan diam-diam, membuat proyek internal terbit sebagai proyek publik.
// Lebih baik impor gagal dan orangnya memutuskan.
func readProjects(path string) ([]projectInput, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Dibaca dua tahap: daftar entri mentah dulu, supaya galat field asing bisa
	// menyebut entri ke berapa — pesan `json: unknown field` sendirian tidak.
	entries, err := rawEntries(raw)
	if err != nil {
		return nil, err
	}

	projects := make([]projectInput, 0, len(entries))
	for i, entry := range entries {
		dec := json.NewDecoder(bytes.NewReader(entry))
		dec.DisallowUnknownFields()

		var p projectInput
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("proyek #%d: %w", i+1, err)
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// rawEntries memisahkan pembungkus dari isinya, menerima objek berkunci
// "projects" maupun array telanjang.
func rawEntries(raw []byte) ([]json.RawMessage, error) {
	var wrapped struct {
		Projects []json.RawMessage `json:"projects"`
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err := dec.Decode(&wrapped)
	switch {
	case err == nil:
		return wrapped.Projects, nil
	case strings.Contains(err.Error(), "unknown field"):
		// Blok tambahan di akar berkas (mis. `_internal` berisi catatan yang
		// tidak untuk publik) ditolak, bukan dilewati tanpa suara.
		return nil, fmt.Errorf("akar berkas: %w", err)
	}

	var bare []json.RawMessage
	if err := json.Unmarshal(raw, &bare); err != nil {
		return nil, fmt.Errorf("JSON tidak dikenali; harap berupa {\"projects\": [...]} atau array proyek: %w", err)
	}
	return bare, nil
}

// resolveTags menukar nama tag menjadi UUID, membuat tag yang belum ada.
//
// Pencocokan case-insensitive supaya "Go" dan "go" tidak menghasilkan dua baris.
func resolveTags(ctx context.Context, gormDB *gorm.DB, refs []tagRef) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(refs))

	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		tipe := strings.TrimSpace(ref.Type)
		if tipe == "" {
			// Tanpa tipe, tag tetap dipakai di kartu proyek tapi tidak muncul
			// terkelompok di grid keahlian. "lainnya" membuatnya tetap terlihat.
			tipe = "lainnya"
		}

		var tag models.Tag
		err := gormDB.WithContext(ctx).
			Where("LOWER(name) = ?", strings.ToLower(name)).
			First(&tag).Error

		switch {
		case err == nil:
			ids = append(ids, tag.ID)

		case errors.Is(err, gorm.ErrRecordNotFound):
			tag = models.Tag{Name: name, Type: tipe}
			if err := gormDB.WithContext(ctx).Create(&tag).Error; err != nil {
				return nil, fmt.Errorf("gagal membuat tag %q: %w", name, err)
			}
			log.Info().Str("tag", name).Str("type", tipe).Msg("tag baru dibuat")
			ids = append(ids, tag.ID)

		default:
			return nil, fmt.Errorf("gagal mencari tag %q: %w", name, err)
		}
	}

	return ids, nil
}

// toModel memetakan input berkas ke struct GORM (hanya field skalar; relasi
// ditangani repository lewat ProjectWriteInput).
func toModel(p projectInput) models.Project {
	m := models.Project{
		Title:         strings.TrimSpace(p.Title),
		Slug:          strings.TrimSpace(p.Slug),
		ShortDesc:     p.ShortDesc,
		LongDesc:      p.LongDesc,
		CoverImageURL: p.CoverImageURL,
		Category:      p.Category,
		Timeline:      p.Timeline,
		Role:          p.Role,
		Challenge:     p.Challenge,
		Solution:      p.Solution,
		Results:       p.Results,
		DemoURL:       p.DemoURL,
		RepoURL:       p.RepoURL,
		IsFeatured:    p.IsFeatured,
		SortOrder:     p.SortOrder,
	}

	// technicalDetails disimpan sebagai kolom JSON; objek kosong lebih aman
	// daripada NULL karena frontend membacanya sebagai map.
	details := p.TechnicalDetails
	if details == nil {
		details = map[string]any{}
	}
	if b, err := json.Marshal(details); err == nil {
		m.TechnicalDetails = datatypes.JSON(b)
	}

	return m
}
