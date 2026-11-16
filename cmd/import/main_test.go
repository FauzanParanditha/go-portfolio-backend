package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tulisBerkas menaruh isi JSON di berkas sementara dan mengembalikan path-nya.
func tulisBerkas(t *testing.T, isi string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "impor.json")
	if err := os.WriteFile(path, []byte(isi), 0o600); err != nil {
		t.Fatalf("gagal menulis berkas uji: %v", err)
	}
	return path
}

func TestReadProjectsMenerimaDuaBentuk(t *testing.T) {
	entri := `{"title":"Contoh","slug":"contoh","shortDesc":"ringkas"}`

	kasus := map[string]string{
		"objek berkunci projects": `{"projects":[` + entri + `]}`,
		"array telanjang":         `[` + entri + `]`,
	}

	for nama, isi := range kasus {
		t.Run(nama, func(t *testing.T) {
			projects, err := readProjects(tulisBerkas(t, isi))
			if err != nil {
				t.Fatalf("tidak mengharapkan galat: %v", err)
			}
			if len(projects) != 1 || projects[0].Slug != "contoh" {
				t.Fatalf("hasil tidak sesuai: %+v", projects)
			}
		})
	}
}

// Inti dari pengerasan ini: `visibility: internal` pernah lolos tanpa suara,
// sehingga proyek internal akan terbit sebagai proyek publik.
func TestReadProjectsMenolakFieldDiLuarSkema(t *testing.T) {
	isi := `{"projects":[
		{"title":"A","slug":"a","shortDesc":"x"},
		{"title":"B","slug":"b","shortDesc":"y","visibility":"internal"}
	]}`

	_, err := readProjects(tulisBerkas(t, isi))
	if err == nil {
		t.Fatal("field di luar skema seharusnya menolak impor, bukan diabaikan")
	}
	// Pesannya harus menyebut nama field DAN entri ke berapa.
	for _, wajib := range []string{"visibility", "#2"} {
		if !strings.Contains(err.Error(), wajib) {
			t.Errorf("pesan galat tidak menyebut %q: %v", wajib, err)
		}
	}
}

func TestReadProjectsMenolakBlokTambahanDiAkar(t *testing.T) {
	isi := `{"projects":[{"title":"A","slug":"a","shortDesc":"x"}],
	         "_internal":{"catatan":"jangan dipublikasikan"}}`

	_, err := readProjects(tulisBerkas(t, isi))
	if err == nil {
		t.Fatal("blok tambahan di akar seharusnya menolak impor")
	}
	if !strings.Contains(err.Error(), "_internal") {
		t.Errorf("pesan galat tidak menyebut blok yang bermasalah: %v", err)
	}
}

func TestReadProjectsJSONTidakDikenali(t *testing.T) {
	if _, err := readProjects(tulisBerkas(t, `"bukan objek maupun array"`)); err == nil {
		t.Fatal("JSON dengan bentuk lain seharusnya ditolak")
	}
}

func TestValidateFieldWajib(t *testing.T) {
	err := projectInput{Title: "Ada"}.validate(0)
	if err == nil {
		t.Fatal("slug dan shortDesc kosong seharusnya gagal validasi")
	}
	for _, wajib := range []string{"slug", "shortDesc"} {
		if !strings.Contains(err.Error(), wajib) {
			t.Errorf("pesan galat tidak menyebut %q: %v", wajib, err)
		}
	}
}

// Impor tidak lewat DTO handler; tanpa pemeriksaan eksplisit, berkas impor jadi
// jalan pintas menyimpan `javascript:` ke field yang dirender sebagai link.
func TestValidateMenolakURLBerbahaya(t *testing.T) {
	js := "javascript:alert(1)"
	data := "data:text/html,<script>alert(1)</script>"
	dasar := projectInput{Title: "A", Slug: "a", ShortDesc: "x"}

	kasus := map[string]struct {
		ubah  func(p *projectInput)
		field string
	}{
		"demoUrl javascript": {func(p *projectInput) { p.DemoURL = &js }, "demoUrl"},
		"repoUrl data":       {func(p *projectInput) { p.RepoURL = &data }, "repoUrl"},
		"cover javascript":   {func(p *projectInput) { p.CoverImageURL = js }, "coverImageUrl"},
		"screenshot ftp": {func(p *projectInput) {
			p.Screenshots = []string{"https://cdn.contoh.com/1.png", "ftp://contoh.com/2.png"}
		}, "screenshots[1]"},
	}

	for nama, k := range kasus {
		t.Run(nama, func(t *testing.T) {
			p := dasar
			k.ubah(&p)
			err := p.validate(0)
			if err == nil {
				t.Fatal("URL berbahaya seharusnya menolak impor")
			}
			if !strings.Contains(err.Error(), k.field) {
				t.Errorf("pesan galat tidak menyebut %q: %v", k.field, err)
			}
		})
	}
}

func TestValidateMenerimaURLAman(t *testing.T) {
	repo := "https://github.com/contoh/repo"
	kosong := ""
	p := projectInput{
		Title: "A", Slug: "a", ShortDesc: "x",
		DemoURL:       &kosong, // kosong di impor = tidak ada demo
		RepoURL:       &repo,
		CoverImageURL: "/images/cover.webp",
		Screenshots:   []string{"https://cdn.contoh.com/1.png"},
	}
	if err := p.validate(0); err != nil {
		t.Fatalf("URL aman seharusnya lolos: %v", err)
	}
}
