// Package voicemail synchronizes carrier visual voicemail into local storage.
package voicemail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"sim7600d/internal/modem"
	"sim7600d/internal/store"
)

type Credentials struct {
	Server   string
	Port     string
	Username string
	Password string
}

type RemoteMessage struct {
	SourceID    string
	From        string
	ReceivedAt  time.Time
	DurationMS  int
	ContentType string
	Audio       []byte
	Transcript  string
}

type Fetcher interface {
	Fetch(context.Context, Credentials) ([]RemoteMessage, error)
}

type Transcoder interface {
	ToWAV(context.Context, []byte) ([]byte, error)
}

type SyncResult struct {
	Fetched int `json:"fetched"`
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type Service struct {
	store      *store.Store
	modem      modem.Modem
	fetcher    Fetcher
	transcoder Transcoder
	mu         sync.Mutex
}

func New(st *store.Store, m modem.Modem) *Service {
	service := NewWithFetcher(st, m, NewIMAPFetcher())
	service.transcoder = FFmpegTranscoder{Path: "ffmpeg"}
	return service
}

func NewWithFetcher(st *store.Store, m modem.Modem, fetcher Fetcher) *Service {
	return &Service{store: st, modem: m, fetcher: fetcher}
}

func (s *Service) Sync(ctx context.Context) (SyncResult, error) {
	if s == nil || s.store == nil || s.modem == nil || s.fetcher == nil {
		return SyncResult{}, errors.New("voicemail sync is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	messages, err := s.modem.ListInbound(ctx, "", 500)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list voicemail notifications: %w", err)
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ReceivedAt.After(messages[j].ReceivedAt) })
	var credentials Credentials
	var sourceSMSID string
	for _, message := range messages {
		parsed, parseErr := ParseMailboxUpdate(message.Body)
		if parseErr == nil {
			credentials = parsed
			sourceSMSID = message.ID
			break
		}
	}
	if credentials.Server == "" {
		return SyncResult{}, errors.New("no supported voicemail mailbox update SMS found")
	}
	remote, err := s.fetcher.Fetch(ctx, credentials)
	if err != nil {
		return SyncResult{}, fmt.Errorf("fetch voicemail mailbox: %w", err)
	}
	result := SyncResult{Fetched: len(remote)}
	for _, message := range remote {
		if strings.EqualFold(message.ContentType, "audio/amr") {
			if s.transcoder == nil {
				return result, errors.New("AMR voicemail requires ffmpeg transcoding")
			}
			message.Audio, err = s.transcoder.ToWAV(ctx, message.Audio)
			if err != nil {
				return result, fmt.Errorf("transcode voicemail %s: %w", message.SourceID, err)
			}
			message.ContentType = "audio/wav"
		}
		_, existingErr := s.store.GetVoicemailBySourceID(ctx, message.SourceID, false)
		_, err := s.store.UpsertVoicemail(ctx, store.Voicemail{
			SourceID: message.SourceID, FromAddr: message.From, ReceivedAt: message.ReceivedAt,
			DurationMS: message.DurationMS, ContentType: message.ContentType, Audio: message.Audio,
			Transcript: message.Transcript, SourceSMSID: sourceSMSID,
		})
		if errors.Is(err, store.ErrVoicemailDeleted) {
			result.Skipped++
			continue
		}
		if err != nil {
			return result, fmt.Errorf("store voicemail %s: %w", message.SourceID, err)
		}
		if errors.Is(existingErr, sql.ErrNoRows) {
			result.Added++
			stored, _ := s.store.GetVoicemailBySourceID(ctx, message.SourceID, false)
			_, _ = s.store.AppendEvent(ctx, store.Event{Kind: "voicemail.arrived", RefKind: "voicemail", RefID: stored.ID})
		} else {
			result.Updated++
		}
	}
	return result, nil
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	syncNow := func() {
		child, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		result, err := s.Sync(child)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("voicemail sync", "err", err)
			return
		}
		if result.Added > 0 || result.Updated > 0 {
			slog.Info("voicemail sync complete", "fetched", result.Fetched, "added", result.Added, "updated", result.Updated, "skipped", result.Skipped)
		}
	}
	syncNow()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncNow()
		}
	}
}

func ParseMailboxUpdate(body string) (Credentials, error) {
	const prefix = "MBOXUPDATE?;"
	if !strings.HasPrefix(strings.TrimSpace(body), prefix) {
		return Credentials{}, errors.New("not a mailbox update")
	}
	fields := make(map[string]string)
	for _, part := range strings.Split(strings.TrimPrefix(strings.TrimSpace(body), prefix), ";") {
		key, value, ok := strings.Cut(part, "=")
		if ok && key != "" {
			fields[key] = value
		}
	}
	credentials := Credentials{Server: strings.ToLower(fields["server"]), Port: fields["port"], Username: fields["name"], Password: fields["pw"]}
	if credentials.Port != "993" {
		return Credentials{}, errors.New("voicemail mailbox must use IMAPS port 993")
	}
	if credentials.Server == "" || (!strings.HasSuffix(credentials.Server, ".vvm.mstore.msg.t-mobile.com") && credentials.Server != "vvm.mstore.msg.t-mobile.com") {
		return Credentials{}, errors.New("unsupported voicemail mailbox host")
	}
	if credentials.Username == "" || credentials.Password == "" {
		return Credentials{}, errors.New("voicemail mailbox credentials are incomplete")
	}
	return credentials, nil
}
