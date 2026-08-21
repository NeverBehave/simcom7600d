//go:build hardware

package voicemail

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLiveMailboxFetch exercises the same read-only IMAPS and MIME path used by
// production. The provisioning SMS stays in process memory; no credential or
// fetched audio is logged or written to disk.
func TestLiveMailboxFetch(t *testing.T) {
	provisioning := os.Getenv("SIM7600D_VOICEMAIL_MBOXUPDATE")
	fetchOnly := os.Getenv("SIM7600D_VOICEMAIL_FETCH_ONLY") == "1"
	if provisioning == "" {
		t.Skip("set SIM7600D_VOICEMAIL_MBOXUPDATE to an authorized provisioning SMS")
	}
	credentials, err := ParseMailboxUpdate(provisioning)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	messages, err := NewIMAPFetcher().Fetch(ctx, credentials)
	require.NoError(t, err)
	require.NotEmpty(t, messages, "carrier mailbox contained no playable voicemail")
	for _, message := range messages {
		require.NotEmpty(t, message.SourceID)
		require.True(t, strings.HasPrefix(message.ContentType, "audio/"))
		require.NotEmpty(t, message.Audio)
		if strings.EqualFold(message.ContentType, "audio/amr") && !fetchOnly {
			wav, err := (FFmpegTranscoder{Path: "ffmpeg"}).ToWAV(ctx, message.Audio)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(wav), "RIFF"))
		}
	}
	t.Logf("fetched and validated %d playable voicemail(s)", len(messages))
}
