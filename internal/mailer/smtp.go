package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
)

// ErrNotConfigured dikembalikan saat pengiriman diminta tapi SMTP belum diatur.
var ErrNotConfigured = errors.New("SMTP belum dikonfigurasi")

// smtpMailer mengirim email lewat server SMTP.
//
// Port 465 memakai TLS implisit (koneksi TLS sejak awal); port lain (mis. 587)
// memakai koneksi polos lalu di-upgrade dengan STARTTLS. Pengiriman tanpa TLS
// TIDAK didukung — kredensial SMTP dan tautan reset tidak boleh melintas polos.
type smtpMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
}

func newSMTPMailer(cfg *config.Config) Mailer {
	return &smtpMailer{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		username: cfg.SMTPUsername,
		password: cfg.SMTPPassword,
		from:     cfg.SMTPFrom,
		fromName: cfg.SMTPFromName,
	}
}

func (m *smtpMailer) Enabled() bool { return true }

func (m *smtpMailer) Send(ctx context.Context, to, subject, body string) error {
	addr := net.JoinHostPort(m.host, fmt.Sprintf("%d", m.port))

	// Dial dengan deadline dari ctx supaya server SMTP yang menggantung tidak
	// menahan request HTTP tanpa batas.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var (
		conn net.Conn
		err  error
	)
	if m.port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: m.host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("gagal menghubungi server SMTP: %w", err)
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("gagal membuka sesi SMTP: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if m.port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("server SMTP tidak mendukung STARTTLS; pengiriman tanpa enkripsi ditolak")
		}
		if err := client.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return fmt.Errorf("STARTTLS gagal: %w", err)
		}
	}

	if m.username != "" {
		auth := smtp.PlainAuth("", m.username, m.password, m.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("autentikasi SMTP gagal: %w", err)
		}
	}

	if err := client.Mail(m.from); err != nil {
		return fmt.Errorf("perintah MAIL FROM gagal: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("perintah RCPT TO gagal: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("perintah DATA gagal: %w", err)
	}
	if _, err := w.Write([]byte(m.buildMessage(to, subject, body))); err != nil {
		_ = w.Close()
		return fmt.Errorf("gagal menulis isi email: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gagal menutup isi email: %w", err)
	}

	return nil
}

// buildMessage menyusun pesan RFC 5322 sederhana (teks polos, UTF-8).
// Header dibersihkan dari CR/LF untuk mencegah header injection lewat nilai
// yang berasal dari luar (mis. alamat tujuan).
func (m *smtpMailer) buildMessage(to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + sanitizeHeader(m.fromName) + " <" + sanitizeHeader(m.from) + ">\r\n")
	sb.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	sb.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return sb.String()
}

// sanitizeHeader membuang CR/LF agar nilai tidak bisa menyuntik header tambahan.
func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}
