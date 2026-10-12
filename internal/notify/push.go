package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Flixarr/flixarr/internal/webpush"
)

const keyVAPID = "push_vapid" // the server's VAPID key pair (encrypted)

// VAPID returns the server's Web Push key pair, making it the first time.
func (n *Notifier) VAPID() (webpush.Keys, error) {
	var k webpush.Keys
	if raw := n.Store.SecretSetting(keyVAPID, ""); raw != "" {
		if err := jsonUnmarshal(raw, &k); err == nil && k.Public != "" {
			return k, nil
		}
	}
	k, err := webpush.NewKeys()
	if err != nil {
		return k, err
	}
	return k, n.Store.SetSecretSetting(keyVAPID, jsonString(k))
}

// push sends to every device of the given people; subscriptions the browser dropped are forgotten.
func (n *Notifier) push(ctx context.Context, userIDs []int64, m Message) error {
	keys, err := n.VAPID()
	if err != nil {
		return err
	}
	subject := n.Store.Setting("app_url", "")
	if subject == "" {
		subject = "https://flixarr.app"
	}
	payload := map[string]any{"title": m.Title, "body": eventText[m.Event].head + ". " + m.Text, "tag": fmt.Sprintf("%s-%d", m.Event, m.Request)}
	if m.Poster != "" {
		payload["image"] = "https://image.tmdb.org/t/p/w342" + m.Poster
	}
	if m.Link != "" {
		payload["url"] = m.Link
	} else if m.Type != "" {
		payload["url"] = fmt.Sprintf("/%s/%d", m.Type, m.TmdbID)
	}
	var firstErr error
	for _, id := range userIDs {
		subs, _ := n.Store.PushSubs(id)
		for _, s := range subs {
			err := webpush.Send(ctx, n.HTTP, keys, subject, webpush.Subscription{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}, payload)
			if errors.Is(err, webpush.ErrGone) {
				n.Store.DeletePushSub(s.Endpoint, 0)
				continue
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
