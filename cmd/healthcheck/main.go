// healthcheck dipakai HEALTHCHECK di image produksi. Image akhirnya distroless
// (tanpa shell, curl, maupun wget), jadi pemeriksaannya harus berupa binary
// sendiri.
//
// Yang diperiksa hanya /healthz (proses hidup dan menjawab), BUKAN /readyz:
// database yang mati tidak boleh membuat kontainer API dianggap rusak lalu
// di-restart — restart tidak memperbaiki database. Keadaan database dilaporkan
// terpisah oleh status.sh lewat /readyz.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		os.Exit(1)
	}
}
