package drivers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"lune/internal/notify"
)

type EmailSMTPDriver struct{}

func NewEmailSMTPDriver() *EmailSMTPDriver { return &EmailSMTPDriver{} }

func (d *EmailSMTPDriver) Type() string { return "email_smtp" }

func (d *EmailSMTPDriver) SecretFields() []string { return []string{"password"} }

func (d *EmailSMTPDriver) DocsURL() string { return "" }

func (d *EmailSMTPDriver) ValidateConfig(raw json.RawMessage) error {
	var cfg struct {
		Host    string   `json:"host"`
		Port    int      `json:"port"`
		From    string   `json:"from"`
		To      []string `json:"to"`
		TLSMode string   `json:"tls_mode"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 || strings.TrimSpace(cfg.From) == "" || len(cfg.To) == 0 {
		return fmt.Errorf("host, port, from and to are required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.TLSMode)) {
	case "starttls", "tls", "none":
	default:
		return fmt.Errorf("tls_mode must be starttls, tls, or none")
	}
	return nil
}

func (d *EmailSMTPDriver) Send(ctx context.Context, n notify.Notification, runtime notify.ChannelRuntime) (notify.Result, error) {
	var cfg struct {
		Host     string   `json:"host"`
		Port     int      `json:"port"`
		Username string   `json:"username"`
		Password string   `json:"password"`
		From     string   `json:"from"`
		To       []string `json:"to"`
		TLSMode  string   `json:"tls_mode"`
	}
	if err := json.Unmarshal(runtime.Config, &cfg); err != nil {
		return notify.Result{}, err
	}
	rendered := runtime.Rendered
	if rendered == nil {
		item, err := notify.RenderNotification(n, runtime.TitleTpl, runtime.BodyTpl)
		if err != nil {
			return notify.Result{}, err
		}
		rendered = &item
	}
	auth := smtpAuth(cfg.Username, cfg.Password, cfg.Host)
	body := strings.Join([]string{
		fmt.Sprintf("From: %s", cfg.From),
		fmt.Sprintf("To: %s", strings.Join(cfg.To, ",")),
		fmt.Sprintf("Subject: %s", rendered.Title),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		rendered.Body,
	}, "\r\n")
	start := time.Now()
	err := sendSMTPMail(ctx, cfg.Host, cfg.Port, normalizeTLSMode(cfg.TLSMode), auth, cfg.From, cfg.To, []byte(body))
	result := notify.Result{LatencyMS: time.Since(start).Milliseconds()}
	if err != nil {
		result.UpstreamCode = "smtp error"
		result.UpstreamMessage = err.Error()
		return result, err
	}
	result.OK = true
	result.UpstreamCode = "smtp accepted"
	result.UpstreamMessage = "accepted"
	return result, nil
}

func normalizeTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "starttls":
		return "starttls"
	case "tls":
		return "tls"
	case "none":
		return "none"
	default:
		return ""
	}
}

func smtpAuth(username, password, host string) smtp.Auth {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil
	}
	return smtp.PlainAuth("", username, password, host)
}

func sendSMTPMail(ctx context.Context, host string, port int, tlsMode string, auth smtp.Auth, from string, to []string, body []byte) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	switch tlsMode {
	case "none":
		return sendSMTPWithClient(ctx, addr, host, false, false, auth, from, to, body)
	case "tls":
		return sendSMTPWithClient(ctx, addr, host, true, false, auth, from, to, body)
	default:
		return sendSMTPWithClient(ctx, addr, host, false, true, auth, from, to, body)
	}
}

func sendSMTPWithClient(ctx context.Context, addr, host string, implicitTLS, useStartTLS bool, auth smtp.Auth, from string, to []string, body []byte) error {
	var (
		conn net.Conn
		err  error
	)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if implicitTLS {
		rawConn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			return dialErr
		}
		tlsConn := tls.Client(rawConn, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return err
		}
		conn = tlsConn
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-stop:
		}
	}()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if !implicitTLS && useStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return err
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
