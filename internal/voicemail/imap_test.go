package voicemail

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseIMAPMessageExtractsAudioMetadataAndTranscript(t *testing.T) {
	audio := append([]byte("#!AMR\n"), make([]byte, 13*3)...)
	encoded := base64.StdEncoding.EncodeToString(audio)
	raw := []byte("Message-ID: <mailbox-42>\r\n" +
		"From: +15551234567@voicemail.invalid\r\n" +
		"Date: Wed, 19 Aug 2026 12:00:00 +0000\r\n" +
		"X-VoiceMessage-Duration: 12\r\n" +
		"Content-Type: multipart/mixed; boundary=vm\r\n\r\n" +
		"--vm\r\nContent-Type: audio/amr\r\nContent-Transfer-Encoding: base64\r\n\r\n" + encoded + "\r\n" +
		"--vm\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=transcription.txt\r\n\r\nCall me back\r\n" +
		"--vm--\r\n")

	got, err := parseIMAPMessage(raw, nil, time.Time{})
	require.NoError(t, err)
	require.Equal(t, "mailbox-42", got.SourceID)
	require.Equal(t, "+15551234567", got.From)
	require.Equal(t, 12000, got.DurationMS)
	require.Equal(t, "audio/amr", got.ContentType)
	require.Equal(t, audio, got.Audio)
	require.Equal(t, "Call me back", got.Transcript)
}

func TestAMRDurationCountsFrames(t *testing.T) {
	frame := append([]byte{0}, make([]byte, 12)...)
	audio := append([]byte("#!AMR\n"), append(frame, frame...)...)
	require.Equal(t, 40, amrDuration(audio))
}
