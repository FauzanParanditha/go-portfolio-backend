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
