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
	switch normalizeTLSMode(cfg.TLSMode) {
	case "starttls", "tls", "none":
	default:
		return fmt.Errorf("tls_mode must be starttls, tls, or none")
	}
	return nil
}

func (d *EmailSMTPDriver) Send(ctx context.Context, n notify.Notification, runtime notify.ChannelRuntime) (notify.Result, error) {
	_ = ctx
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
	rendered, err := notify.RenderNotification(n, runtime.TitleTpl, runtime.BodyTpl)
	if err != nil {
		return notify.Result{}, err
	}
	auth := smtpAuth(cfg.Username, cfg.Password, cfg.Host)
	body := strings.Join([]string{
		fmt.Sprintf("To: %s", strings.Join(cfg.To, ",")),
		fmt.Sprintf("Subject: %s", rendered.Title),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		rendered.Body,
	}, "\r\n")
	start := time.Now()
	err = sendSMTPMail(cfg.Host, cfg.Port, normalizeTLSMode(cfg.TLSMode), auth, cfg.From, cfg.To, []byte(body))
	result := notify.Result{
		LatencyMS:    time.Since(start).Milliseconds(),
		UpstreamCode: "smtp 250",
	}
	if err != nil {
		result.UpstreamCode = "smtp error"
		result.UpstreamMessage = err.Error()
		return result, err
	}
	result.OK = true
	result.UpstreamMessage = "accepted"
	return result, nil
}

func normalizeTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "starttls":
		return "starttls"
	case "tls":
		return "tls"
	case "none":
		return "none"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func smtpAuth(username, password, host string) smtp.Auth {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil
	}
	return smtp.PlainAuth("", username, password, host)
}

func sendSMTPMail(host string, port int, tlsMode string, auth smtp.Auth, from string, to []string, body []byte) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	switch tlsMode {
	case "none":
		if auth == nil {
			return smtp.SendMail(addr, nil, from, to, body)
		}
		return sendSMTPWithClient(addr, host, false, false, auth, from, to, body)
	case "tls":
		return sendSMTPWithClient(addr, host, true, false, auth, from, to, body)
	default:
		return sendSMTPWithClient(addr, host, false, true, auth, from, to, body)
	}
}

func sendSMTPWithClient(addr, host string, implicitTLS, useStartTLS bool, auth smtp.Auth, from string, to []string, body []byte) error {
	var (
		conn net.Conn
		err  error
	)
	if implicitTLS {
		conn, err = tls.Dial("tcp", addr, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if !implicitTLS && useStartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{
				ServerName: host,
				MinVersion: tls.VersionTLS12,
			}); err != nil {
				return err
			}
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
