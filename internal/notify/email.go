package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// sendMail delivers one message to each recipient (separately, so nobody sees the others' addresses).
func sendMail(ctx context.Context, c Config, to []string, m Message) error {
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if c.Security == "tls" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: c.Host})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return errors.New("couldn't reach the SMTP server")
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("the SMTP server didn't answer: %w", err)
	}
	defer cl.Close()
	if c.Security == "starttls" {
		if err := cl.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return fmt.Errorf("the SMTP server refused the sign-in: %w", err)
		}
	}
	for _, rcpt := range to {
		if err := cl.Mail(c.From); err != nil {
			return fmt.Errorf("the SMTP server refused the From address: %w", err)
		}
		if err := cl.Rcpt(rcpt); err != nil {
			return fmt.Errorf("the SMTP server refused %s: %w", rcpt, err)
		}
		w, err := cl.Data()
		if err != nil {
			return err
		}
		w.Write(mailBody(c, rcpt, m))
		if err := w.Close(); err != nil {
			return err
		}
	}
	return cl.Quit()
}

func mailBody(c Config, to string, m Message) []byte {
	head := eventText[m.Event].head
	name := c.FromName
	if name == "" {
		name = "Flixarr"
	}
	subject := head + ": " + m.Title
	var text, page strings.Builder
	text.WriteString(m.Title + "\n\n" + m.Text + "\n")
	if m.Seasons != "" {
		text.WriteString(m.Seasons + "\n")
	}
	page.WriteString(`<div style="font-family:sans-serif;max-width:520px">`)
	page.WriteString(`<p style="color:#f97316;font-size:12px;text-transform:uppercase;letter-spacing:.05em;margin:0">` + html.EscapeString(head) + `</p>`)
	page.WriteString(`<h2 style="margin:4px 0 12px">` + html.EscapeString(m.Title) + `</h2>`)
	if m.Poster != "" {
		page.WriteString(`<img src="https://image.tmdb.org/t/p/w185` + html.EscapeString(m.Poster) + `" alt="" width="120" style="border-radius:8px;float:right;margin-left:12px">`)
	}
	page.WriteString(`<p>` + html.EscapeString(m.Text) + `</p>`)
	if m.Seasons != "" {
		page.WriteString(`<p style="color:#666">` + html.EscapeString(m.Seasons) + `</p>`)
	}
	if m.Link != "" {
		text.WriteString("\n" + m.Link + "\n")
		page.WriteString(`<p><a href="` + html.EscapeString(m.Link) + `" style="color:#f97316">Open in Flixarr</a></p>`)
	}
	page.WriteString(`</div>`)
	boundary := fmt.Sprintf("flixarr-%d", time.Now().UnixNano())
	var b strings.Builder
	b.WriteString("From: " + mime.QEncoding.Encode("utf-8", name) + " <" + c.From + ">\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + text.String() + "\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + page.String() + "\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}
