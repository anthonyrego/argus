package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/icholy/digest"
)

// DahuaSource subscribes to the Dahua/Amcrest event stream at
// /cgi-bin/eventManager.cgi?action=attach. The connection is long-lived; the
// camera pushes a multipart chunk per event (motion, IVS, AI, etc.).
type DahuaSource struct {
	host     string
	username string
	password string
	codes    []string
	client   *http.Client
	log      *slog.Logger
}

func NewDahuaSource(host, username, password string, codes []string, log *slog.Logger) *DahuaSource {
	return &DahuaSource{
		host:     host,
		username: username,
		password: password,
		codes:    codes,
		client: &http.Client{
			Transport: &digest.Transport{
				Username: username,
				Password: password,
			},
		},
		log: log,
	}
}

// Run forwards events to out until ctx is cancelled, reconnecting with
// exponential backoff on transport errors. Returns nil on clean shutdown.
func (d *DahuaSource) Run(ctx context.Context, out chan<- Event) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := d.attach(ctx, out)
		if ctx.Err() != nil {
			return nil
		}
		d.log.Warn("event stream disconnected", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (d *DahuaSource) attach(ctx context.Context, out chan<- Event) error {
	codes := "[All]"
	if len(d.codes) > 0 {
		codes = "[" + strings.Join(d.codes, ",") + "]"
	}
	url := fmt.Sprintf("http://%s/cgi-bin/eventManager.cgi?action=attach&codes=%s", d.host, codes)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return fmt.Errorf("content-type: %w", err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return fmt.Errorf("unexpected content-type %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return fmt.Errorf("no multipart boundary")
	}
	d.log.Info("event stream attached", "host", d.host, "codes", codes)

	mr := multipart.NewReader(resp.Body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read part: %w", err)
		}
		body, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		ev, ok := parseEvent(body)
		if !ok {
			continue
		}
		ev.Time = time.Now()
		select {
		case out <- ev:
		case <-ctx.Done():
			return nil
		}
	}
}

// parseEvent decodes a Dahua event body of the form:
//   Code=<name>;action=<Start|Stop|Pulse>;index=<n>;data={...json...}
// data= is optional. The JSON value can contain ';' and newlines, so we stop
// splitting once we hit data= and treat the remainder as one blob.
func parseEvent(body []byte) (Event, bool) {
	rest := strings.TrimSpace(string(body))
	if rest == "" {
		return Event{}, false
	}
	var ev Event
	for rest != "" {
		if strings.HasPrefix(rest, "data=") {
			raw := strings.TrimSpace(rest[len("data="):])
			if raw != "" {
				var data map[string]any
				if err := json.Unmarshal([]byte(raw), &data); err == nil {
					ev.Data = data
				}
			}
			break
		}
		var kv string
		if i := strings.IndexByte(rest, ';'); i >= 0 {
			kv, rest = rest[:i], rest[i+1:]
		} else {
			kv, rest = rest, ""
		}
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k, v := strings.TrimSpace(kv[:eq]), strings.TrimSpace(kv[eq+1:])
		switch k {
		case "Code":
			ev.Code = v
		case "action":
			ev.Action = v
		case "index":
			ev.Index, _ = strconv.Atoi(v)
		}
	}
	if ev.Code == "" {
		return Event{}, false
	}
	return ev, true
}
