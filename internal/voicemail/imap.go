package voicemail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

const maxVoicemailBytes = 32 << 20

type IMAPFetcher struct {
	dial func(string, *imapclient.Options) (*imapclient.Client, error)
}

func NewIMAPFetcher() *IMAPFetcher {
	return &IMAPFetcher{dial: imapclient.DialTLS}
}

func (f *IMAPFetcher) Fetch(ctx context.Context, credentials Credentials) ([]RemoteMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := f.dial(net.JoinHostPort(credentials.Server, credentials.Port), &imapclient.Options{
		Dialer: &net.Dialer{Timeout: 15 * time.Second},
	})
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.Login(credentials.Username, credentials.Password).Wait(); err != nil {
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}
	selected, err := client.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return nil, fmt.Errorf("select inbox: %w", err)
	}
	start := uint32(1)
	if selected.NumMessages > 100 {
		start = selected.NumMessages - 99
	}
	section := &imap.FetchItemBodySection{Peek: true}
	items := make([]RemoteMessage, 0, selected.NumMessages-start+1)
	for seq := start; seq <= selected.NumMessages && selected.NumMessages > 0; seq++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		messages, err := client.Fetch(imap.SeqSetNum(seq), &imap.FetchOptions{
			UID: true, Envelope: true, InternalDate: true,
			BodySection: []*imap.FetchItemBodySection{section},
		}).Collect()
		if err != nil {
			return nil, fmt.Errorf("fetch message %d: %w", seq, err)
		}
		for _, message := range messages {
			raw := message.FindBodySection(section)
			if len(raw) == 0 {
				continue
			}
			parsed, err := parseIMAPMessage(raw, message.Envelope, message.InternalDate)
			if err != nil {
				continue
			}
			if parsed.SourceID == "" {
				parsed.SourceID = fmt.Sprintf("%s:%d:%d", credentials.Server, selected.UIDValidity, message.UID)
			}
			items = append(items, parsed)
		}
	}
	return items, nil
}

type mimeAudio struct {
	contentType string
	data        []byte
	durationMS  int
}

func parseIMAPMessage(raw []byte, envelope *imap.Envelope, internalDate time.Time) (RemoteMessage, error) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return RemoteMessage{}, err
	}
	header := textproto.MIMEHeader(message.Header)
	audio, transcript, err := extractMIME(header, message.Body)
	if err != nil {
		return RemoteMessage{}, err
	}
	if len(audio.data) == 0 || !strings.HasPrefix(audio.contentType, "audio/") {
		return RemoteMessage{}, errors.New("message has no voicemail audio")
	}
	received, _ := message.Header.Date()
	if received.IsZero() && envelope != nil {
		received = envelope.Date
	}
	if received.IsZero() {
		received = internalDate
	}
	if received.IsZero() {
		received = time.Now().UTC()
	}
	from := firstNonEmpty(
		message.Header.Get("X-VoiceMessage-Caller-ID"),
		message.Header.Get("X-Caller-Number"),
		message.Header.Get("X-Original-From"),
		message.Header.Get("From"),
	)
	duration := firstDuration(
		message.Header.Get("X-VoiceMessage-Duration"),
		message.Header.Get("X-Voicemail-Duration"),
		message.Header.Get("Content-Duration"),
	)
	if duration == 0 {
		duration = audio.durationMS
	}
	if duration == 0 {
		duration = amrDuration(audio.data)
	}
	return RemoteMessage{
		SourceID: strings.Trim(message.Header.Get("Message-ID"), "<> "),
		From:     normalizeCaller(from), ReceivedAt: received.UTC(), DurationMS: duration,
		ContentType: audio.contentType, Audio: audio.data,
		Transcript: firstNonEmpty(message.Header.Get("X-VoiceMessage-Transcription"), transcript),
	}, nil
}

func extractMIME(header textproto.MIMEHeader, body io.Reader) (mimeAudio, string, error) {
	contentType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || contentType == "" {
		contentType = "text/plain"
	}
	contentType = strings.ToLower(contentType)
	if strings.HasPrefix(contentType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return mimeAudio{}, "", errors.New("multipart voicemail has no boundary")
		}
		reader := multipart.NewReader(body, boundary)
		var audio mimeAudio
		var transcript string
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return mimeAudio{}, "", err
			}
			partAudio, partTranscript, err := extractMIME(part.Header, part)
			part.Close()
			if err != nil {
				continue
			}
			if len(audio.data) == 0 && len(partAudio.data) > 0 {
				audio = partAudio
			}
			if transcript == "" && partTranscript != "" {
				transcript = partTranscript
			}
		}
		return audio, transcript, nil
	}
	if contentType == "message/rfc822" {
		nested, err := mail.ReadMessage(body)
		if err != nil {
			return mimeAudio{}, "", err
		}
		return extractMIME(textproto.MIMEHeader(nested.Header), nested.Body)
	}
	decoded := transferDecodedReader(header.Get("Content-Transfer-Encoding"), body)
	data, err := io.ReadAll(io.LimitReader(decoded, maxVoicemailBytes+1))
	if err != nil {
		return mimeAudio{}, "", err
	}
	if len(data) > maxVoicemailBytes {
		return mimeAudio{}, "", errors.New("voicemail MIME part is too large")
	}
	if strings.HasPrefix(contentType, "audio/") {
		return mimeAudio{contentType: contentType, data: data, durationMS: firstDuration(header.Get("Content-Duration"), params["duration"])}, "", nil
	}
	if contentType == "text/plain" && transcriptPart(header) {
		return mimeAudio{}, strings.TrimSpace(string(data)), nil
	}
	return mimeAudio{}, "", nil
}

func transferDecodedReader(encoding string, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	default:
		return body
	}
}

func transcriptPart(header textproto.MIMEHeader) bool {
	disposition, params, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	name := strings.ToLower(params["filename"])
	return strings.Contains(strings.ToLower(disposition), "attachment") &&
		(strings.Contains(name, "transcript") || strings.Contains(name, "transcription"))
}

var phonePattern = regexp.MustCompile(`\+?[0-9][0-9(). -]{5,}[0-9]`)

func normalizeCaller(value string) string {
	if address, err := mail.ParseAddress(value); err == nil {
		value = address.Address
		if local, _, ok := strings.Cut(value, "@"); ok {
			value = local
		}
	}
	match := phonePattern.FindString(value)
	if match == "" {
		return value
	}
	plus := strings.HasPrefix(strings.TrimSpace(match), "+")
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, match)
	if plus {
		return "+" + digits
	}
	return digits
}

func firstDuration(values ...string) int {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if seconds, err := strconv.ParseFloat(value, 64); err == nil {
			if seconds > 0 {
				return int(seconds * 1000)
			}
			continue
		}
		parts := strings.Split(value, ":")
		if len(parts) == 2 || len(parts) == 3 {
			var seconds float64
			for _, part := range parts {
				n, err := strconv.ParseFloat(part, 64)
				if err != nil {
					seconds = 0
					break
				}
				seconds = seconds*60 + n
			}
			if seconds > 0 {
				return int(seconds * 1000)
			}
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func amrDuration(audio []byte) int {
	var sizes []int
	var offset int
	switch {
	case bytes.HasPrefix(audio, []byte("#!AMR\n")):
		offset, sizes = 6, []int{13, 14, 16, 18, 20, 21, 27, 32, 6}
	case bytes.HasPrefix(audio, []byte("#!AMR-WB\n")):
		offset, sizes = 9, []int{18, 24, 33, 37, 41, 47, 51, 59, 61, 6}
	default:
		return 0
	}
	frames := 0
	for offset < len(audio) {
		frameType := int((audio[offset] >> 3) & 0x0f)
		if frameType == 15 {
			offset++
			frames++
			continue
		}
		if frameType >= len(sizes) || offset+sizes[frameType] > len(audio) {
			break
		}
		offset += sizes[frameType]
		frames++
	}
	return frames * 20
}
