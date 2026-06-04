// Package denylist menyediakan daftar token (berbasis JWT ID / jti) yang
// dicabut sebelum kedaluwarsa — dipakai untuk membuat logout benar-benar
// mematikan token, bukan menunggu sampai `exp`.
package denylist

import (
	"sync"
	"time"
)

// Denylist adalah kontrak penyimpanan jti yang dicabut.
// Implementasi default-nya in-memory (lihat Memory), tetapi sengaja dibuat
// interface agar mudah diganti dengan implementasi DB/Redis (multi-instance).
type Denylist interface {
	// Revoke menandai jti sebagai dicabut sampai waktu exp (saat token itu
	// memang kedaluwarsa). Pemanggilan dengan jti kosong diabaikan.
	Revoke(jti string, exp time.Time)
	// IsRevoked mengembalikan true jika jti masih dalam masa cabut & belum lewat exp.
	IsRevoked(jti string) bool
}

// Memory adalah Denylist in-memory aman-konkuren dengan pembersihan otomatis.
//
// Keterbatasan (sadar): state hilang saat proses restart, dan tidak dibagikan
// antar-instance. Untuk deployment single-instance (portfolio) ini memadai;
// untuk multi-instance ganti dengan implementasi berbasis DB/Redis.
type Memory struct {
	mu sync.RWMutex
	m  map[string]time.Time // jti -> exp
}

// NewMemory membuat denylist in-memory dan menjalankan janitor pembersih
// entri kedaluwarsa secara periodik.
func NewMemory() *Memory {
	d := &Memory{m: make(map[string]time.Time)}
	go d.janitor(10 * time.Minute)
	return d
}

func (d *Memory) Revoke(jti string, exp time.Time) {
	if jti == "" {
		return
	}
	d.mu.Lock()
	d.m[jti] = exp
	d.mu.Unlock()
}

func (d *Memory) IsRevoked(jti string) bool {
	if jti == "" {
		return false
	}

	d.mu.RLock()
	exp, ok := d.m[jti]
	d.mu.RUnlock()
	if !ok {
		return false
	}

	// Entri sudah lewat exp: token-nya juga sudah mati, perlakukan sebagai
	// tidak-dicabut dan bersihkan secara lazy.
	if time.Now().After(exp) {
		d.mu.Lock()
		delete(d.m, jti)
		d.mu.Unlock()
		return false
	}
	return true
}

// janitor membersihkan entri yang sudah lewat exp setiap interval.
func (d *Memory) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		d.mu.Lock()
		for jti, exp := range d.m {
			if now.After(exp) {
				delete(d.m, jti)
			}
		}
		d.mu.Unlock()
	}
}
