package config_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Di produksi, kontainer API hanya menerima env yang DITULIS di daftar
// `environment:` layanan api pada deploy/docker-compose.yml (bukan seluruh
// deploy/.env, yang juga memuat sandi database dan token alarm). Akibatnya,
// kunci env baru yang dibaca aplikasi tetapi lupa ditambahkan ke daftar itu
// diam-diam tidak pernah sampai: "sudah saya isi di .env, kok tidak terbaca".
//
// Test ini membaca sumber Go server (internal/ dan cmd/api) dan menuntut tiap
// kunci env yang dibacanya ada di daftar tersebut.

var (
	polaBacaEnv = regexp.MustCompile(`(?:GetEnv(?:Int|Bool)?|os\.Getenv|os\.LookupEnv)\("([A-Z][A-Z0-9_]*)"`)
	polaDaftar  = regexp.MustCompile(`^\s+- ([A-Z][A-Z0-9_]*)(=.*)?$`)
)

func kunciDibacaAplikasi(t *testing.T, akar string) []string {
	t.Helper()

	set := map[string]bool{}
	for _, dir := range []string{"internal", filepath.Join("cmd", "api")} {
		err := filepath.WalkDir(filepath.Join(akar, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			isi, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range polaBacaEnv.FindAllStringSubmatch(string(isi), -1) {
				set[m[1]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("membaca sumber %s: %v", dir, err)
		}
	}

	kunci := make([]string, 0, len(set))
	for k := range set {
		kunci = append(kunci, k)
	}
	sort.Strings(kunci)
	return kunci
}

// kunciDiCompose mengembalikan nama env di blok `environment:` layanan api.
func kunciDiCompose(t *testing.T, akar string) map[string]bool {
	t.Helper()

	isi, err := os.ReadFile(filepath.Join(akar, "deploy", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("membaca compose: %v", err)
	}

	set := map[string]bool{}
	diApi, diEnv := false, false
	for _, baris := range strings.Split(string(isi), "\n") {
		switch {
		case strings.HasPrefix(baris, "  api:"):
			diApi = true
		case diApi && strings.HasPrefix(baris, "  ") && !strings.HasPrefix(baris, "   ") && strings.TrimSpace(baris) != "":
			// Layanan berikutnya (indentasi dua spasi) mengakhiri blok api.
			diApi, diEnv = false, false
		case diApi && strings.TrimSpace(baris) == "environment:":
			diEnv = true
		case diApi && diEnv:
			if m := polaDaftar.FindStringSubmatch(baris); m != nil {
				set[m[1]] = true
			} else if t := strings.TrimSpace(baris); t != "" && !strings.HasPrefix(t, "#") {
				diEnv = false
			}
		}
	}
	return set
}

func TestSemuaEnvAplikasiDiteruskanKeKontainer(t *testing.T) {
	akar := filepath.Join("..", "..")

	dibaca := kunciDibacaAplikasi(t, akar)
	diteruskan := kunciDiCompose(t, akar)

	// Jaga agar test tidak lulus kosong bila pola atau struktur berkas berubah.
	if len(dibaca) < 10 {
		t.Fatalf("hanya menemukan %d kunci env di sumber — pola pencarian tidak lagi cocok", len(dibaca))
	}
	if len(diteruskan) < 10 {
		t.Fatalf("hanya menemukan %d kunci di environment layanan api — struktur compose berubah?", len(diteruskan))
	}

	for _, k := range dibaca {
		if !diteruskan[k] {
			t.Errorf("%s dibaca aplikasi tetapi tidak ada di `environment:` layanan api "+
				"(deploy/docker-compose.yml) — di produksi nilainya tidak akan pernah sampai", k)
		}
	}
}
