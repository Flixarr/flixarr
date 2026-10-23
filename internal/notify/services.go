package notify

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
)

// Notification services that take a short text message: Telegram, Slack, Pushover, Gotify, ntfy and
// Pushbullet. Each needs a token (kept encrypted like every channel secret) and some a target (a chat,
// a user, a topic) or the address of a self-hosted server.

// The services' own addresses (changed in tests).
var (
	telegramAPI   = "https://api.telegram.org"
	pushoverAPI   = "https://api.pushover.net"
	pushbulletAPI = "https://api.pushbullet.com"
)

// Simple are the channels that send plain text (Settings shows the same few fields for each).
var Simple = []string{"telegram", "slack", "pushover", "gotify", "ntfy", "pushbullet"}

// textOf is a message as a heading and body text, the same for every service.
func textOf(m Message) (head, body string) {
	head = eventText[m.Event].head + ": " + m.Title
	lines := []string{}
	if m.Text != "" {
		lines = append(lines, m.Text)
	}
	if m.Seasons != "" {
		lines = append(lines, m.Seasons)
	}
	if m.Event == "request_failed" && m.Error != "" {
		lines = append(lines, m.Error)
	}
	return head, strings.Join(lines, "\n")
}

// validateSimple checks a text channel's fields when it's on.
func (c Config) validateSimple(ch string) error {
	need := func(v, what string) error {
		if c.On && strings.TrimSpace(v) == "" {
			return fmt.Errorf("enter the %s first", what)
		}
		return nil
	}
	server := func(what string) error {
		if c.Server == "" {
			return need("", what)
		}
		u, err := url.Parse(c.Server)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("that %s doesn't look right; it should be like https://example.com", what)
		}
		return nil
	}
	switch ch {
	case "telegram":
		return errors.Join(need(c.Token, "bot token"), need(c.Target, "chat ID"))
	case "slack":
		if c.Token != "" && !strings.HasPrefix(c.Token, "https://hooks.slack.com/") {
			return errors.New("that isn't a Slack webhook URL; it starts with https://hooks.slack.com/")
		}
		return need(c.Token, "Slack webhook URL")
	case "pushover":
		return errors.Join(need(c.Token, "application token"), need(c.Target, "user key"))
	case "gotify":
		return errors.Join(server("Gotify address"), need(c.Token, "application token"))
	case "ntfy":
		return errors.Join(server("ntfy server"), need(c.Target, "topic"))
	case "pushbullet":
		return need(c.Token, "access token")
	}
	return nil
}

// deliverSimple sends a message through a text channel.
func (n *Notifier) deliverSimple(ctx context.Context, ch string, c Config, m Message) error {
	head, body := textOf(m)
	server := strings.TrimRight(c.Server, "/")
	switch ch {
	case "telegram":
		text := "<b>" + html.EscapeString(head) + "</b>"
		if body != "" {
			text += "\n" + html.EscapeString(body)
		}
		if m.Link != "" {
			text += "\n<a href=\"" + html.EscapeString(m.Link) + "\">Open in Flixarr</a>"
		}
		return n.post(ctx, http.MethodPost, telegramAPI+"/bot"+c.Token+"/sendMessage", "",
			map[string]any{"chat_id": c.Target, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true})
	case "slack":
		text := "*" + head + "*"
		if body != "" {
			text += "\n" + body
		}
		if m.Link != "" {
			text += "\n<" + m.Link + "|Open in Flixarr>"
		}
		return n.post(ctx, http.MethodPost, c.Token, "", map[string]any{"text": text})
	case "pushover":
		msg := map[string]any{"token": c.Token, "user": c.Target, "title": head, "message": orDash(body)}
		if m.Link != "" {
			msg["url"], msg["url_title"] = m.Link, "Open in Flixarr"
		}
		return n.post(ctx, http.MethodPost, pushoverAPI+"/1/messages.json", "", msg)
	case "gotify":
		msg := map[string]any{"title": head, "message": orDash(body), "priority": 5}
		if m.Link != "" {
			msg["extras"] = map[string]any{"client::notification": map[string]any{"click": map[string]any{"url": m.Link}}}
		}
		return n.post(ctx, http.MethodPost, server+"/message?token="+url.QueryEscape(c.Token), "", msg)
	case "ntfy":
		msg := map[string]any{"topic": c.Target, "title": head, "message": orDash(body)}
		if m.Link != "" {
			msg["click"] = m.Link
		}
		auth := ""
		if c.Token != "" {
			auth = "Bearer " + c.Token
		}
		return n.post(ctx, http.MethodPost, server, auth, msg)
	case "pushbullet":
		msg := map[string]any{"type": "note", "title": head, "body": body}
		if m.Link != "" {
			msg["type"], msg["url"] = "link", m.Link
		}
		return n.post(ctx, http.MethodPost, pushbulletAPI+"/v2/pushes", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.Token+":")), msg)
	}
	return errors.New("unknown channel")
}

// orDash: some services refuse an empty message.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
