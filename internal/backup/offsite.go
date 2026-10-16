package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Offsite copies: after each backup, the same file also goes to a second folder (another drive mounted
// into the container) and/or S3-compatible storage (AWS, Backblaze B2, Cloudflare R2, MinIO…). Each place
// keeps the newest Keep() copies, like the backups folder.

// KeyOffsite holds the offsite settings (encrypted: the S3 secret key is in it).
const KeyOffsite = "backup_offsite"

// keyOffsiteLast is how the last copy went: {at, error}.
const keyOffsiteLast = "backup_offsite_last"

// Offsite is where copies go. Empty fields = not used.
type Offsite struct {
	Folder string `json:"folder"`
	S3     S3     `json:"s3"`
}

// S3 is an S3-compatible bucket.
type S3 struct {
	Endpoint  string `json:"endpoint"` // https://s3.us-east-1.amazonaws.com, https://<account>.r2.cloudflarestorage.com…
	Region    string `json:"region"`   // us-east-1 (R2: auto)
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"` // folder inside the bucket, like flixarr/
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

// On says whether an S3 bucket is set up.
func (s S3) On() bool {
	return s.Endpoint != "" && s.Bucket != "" && s.AccessKey != "" && s.SecretKey != ""
}

// OffsiteStatus is how the last offsite copy went.
type OffsiteStatus struct {
	At    string `json:"at"`
	Error string `json:"error,omitempty"`
}

// LoadOffsite returns the offsite settings.
func (m *Manager) LoadOffsite() Offsite {
	var o Offsite
	json.Unmarshal([]byte(m.Store.SecretSetting(KeyOffsite, "")), &o)
	return o
}

// SaveOffsite checks and stores the offsite settings.
func (m *Manager) SaveOffsite(o Offsite) error {
	o.Folder = strings.TrimSpace(o.Folder)
	if o.Folder != "" {
		if !filepath.IsAbs(o.Folder) {
			return errors.New("the folder has to be a full path, like /backups")
		}
		if filepath.Clean(o.Folder) == filepath.Clean(m.dir()) || filepath.Clean(o.Folder) == filepath.Clean(m.DataDir) {
			return errors.New("pick a folder outside Flixarr's data folder; the point is a second copy somewhere else")
		}
		if err := os.MkdirAll(o.Folder, 0o700); err != nil {
			return fmt.Errorf("can't write to that folder: %w", err)
		}
	}
	o.S3.Endpoint = strings.TrimRight(strings.TrimSpace(o.S3.Endpoint), "/")
	if o.S3.Endpoint != "" {
		u, err := url.Parse(o.S3.Endpoint)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errors.New("the S3 endpoint should be like https://s3.us-east-1.amazonaws.com")
		}
		if o.S3.Bucket == "" || o.S3.AccessKey == "" || o.S3.SecretKey == "" {
			return errors.New("enter the bucket, access key and secret key")
		}
		if o.S3.Region == "" {
			o.S3.Region = "us-east-1"
		}
		o.S3.Prefix = strings.Trim(strings.TrimSpace(o.S3.Prefix), "/")
	}
	b, _ := json.Marshal(o)
	return m.Store.SetSecretSetting(KeyOffsite, string(b))
}

// LastOffsite is how the last offsite copy went.
func (m *Manager) LastOffsite() OffsiteStatus {
	var st OffsiteStatus
	json.Unmarshal([]byte(m.Store.Setting(keyOffsiteLast, "")), &st)
	return st
}

// CopyOffsite sends a backup to the offsite places and keeps the newest Keep() copies in each. The result
// is remembered for Settings → Backups.
func (m *Manager) CopyOffsite(ctx context.Context, name string) error {
	o := m.LoadOffsite()
	if o.Folder == "" && !o.S3.On() {
		return nil
	}
	err := m.copyOffsite(ctx, o, name)
	st := OffsiteStatus{At: time.Now().UTC().Format(time.RFC3339)}
	if err != nil {
		st.Error = err.Error()
	}
	b, _ := json.Marshal(st)
	m.Store.SetSetting(keyOffsiteLast, string(b))
	return err
}

func (m *Manager) copyOffsite(ctx context.Context, o Offsite, name string) error {
	src := filepath.Join(m.dir(), name)
	var errs []error
	if o.Folder != "" {
		if err := copyFile(src, filepath.Join(o.Folder, name)); err != nil {
			errs = append(errs, fmt.Errorf("folder: %w", err))
		} else {
			m.pruneFolder(o.Folder)
		}
	}
	if o.S3.On() {
		body, err := os.ReadFile(src)
		if err == nil {
			err = o.S3.put(ctx, name, body)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("S3: %w", err))
		} else if err := o.S3.prune(ctx, m.Keep()); err != nil {
			errs = append(errs, fmt.Errorf("S3 clean up: %w", err))
		}
	}
	return errors.Join(errs...)
}

// pruneFolder keeps the newest Keep() Flixarr backups in the offsite folder (other files are left alone).
func (m *Manager) pruneFolder(dir string) {
	entries, _ := os.ReadDir(dir)
	names := []string{}
	for _, e := range entries {
		if nameRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i, n := range names {
		if i >= m.Keep() {
			os.Remove(filepath.Join(dir, n))
		}
	}
}

// ---------- S3 (signature version 4, path-style addresses) ----------

var s3Client = &http.Client{Timeout: 5 * time.Minute}

func (s S3) key(name string) string {
	if s.Prefix == "" {
		return name
	}
	return s.Prefix + "/" + name
}

func (s S3) put(ctx context.Context, name string, body []byte) error {
	res, err := s.do(ctx, http.MethodPut, "/"+s.Bucket+"/"+s.key(name), nil, body)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// prune deletes all but the newest keep Flixarr backups under the prefix.
func (s S3) prune(ctx context.Context, keep int) error {
	q := url.Values{"list-type": {"2"}}
	if s.Prefix != "" {
		q.Set("prefix", s.Prefix+"/")
	}
	res, err := s.do(ctx, http.MethodGet, "/"+s.Bucket, q, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var list struct {
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&list); err != nil {
		return err
	}
	keys := []string{}
	for _, c := range list.Contents {
		if nameRe.MatchString(filepath.Base(c.Key)) {
			keys = append(keys, c.Key)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	for i, k := range keys {
		if i < keep {
			continue
		}
		if r, err := s.do(ctx, http.MethodDelete, "/"+s.Bucket+"/"+k, nil, nil); err == nil {
			r.Body.Close()
		}
	}
	return nil
}

func (s S3) do(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Response, error) {
	u, _ := url.Parse(s.Endpoint)
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sign(req, body, s.Region, s.AccessKey, s.SecretKey, time.Now().UTC())
	res, err := s3Client.Do(req)
	if err != nil {
		return nil, errors.New("couldn't reach the S3 endpoint")
	}
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 400))
		res.Body.Close()
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		if xml.Unmarshal(msg, &e) == nil && e.Code != "" {
			return nil, fmt.Errorf("%s: %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("it answered %s", res.Status)
	}
	return res, nil
}

// sign adds an AWS Signature Version 4 Authorization header for S3.
func sign(req *http.Request, body []byte, region, accessKey, secretKey string, now time.Time) {
	payload := sha256Hex(body)
	date, stamp := now.Format("20060102"), now.Format("20060102T150405Z")
	req.Header.Set("x-amz-date", stamp)
	req.Header.Set("x-amz-content-sha256", payload)
	host := req.URL.Host
	// Signed headers: host and every x-amz-* / range header set, in order.
	names := []string{"host"}
	vals := map[string]string{"host": host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") || lk == "range" {
			names = append(names, lk)
			vals[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	sort.Strings(names)
	canonHeaders := ""
	for _, n := range names {
		canonHeaders += n + ":" + vals[n] + "\n"
	}
	signed := strings.Join(names, ";")
	canonQuery := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	canonPath := req.URL.EscapedPath()
	if canonPath == "" {
		canonPath = "/"
	}
	canon := strings.Join([]string{req.Method, canonPath, canonQuery, canonHeaders, signed, payload}, "\n")
	scope := date + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + sha256Hex([]byte(canon))
	k := hmacSHA([]byte("AWS4"+secretKey), date)
	k = hmacSHA(k, region)
	k = hmacSHA(k, "s3")
	k = hmacSHA(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func hmacSHA(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
