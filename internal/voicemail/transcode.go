package voicemail

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type FFmpegTranscoder struct {
	Path string
}

func (t FFmpegTranscoder) ToWAV(ctx context.Context, input []byte) ([]byte, error) {
	path := t.Path
	if path == "" {
		path = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-f", "wav", "pipe:1")
	cmd.Stdin = bytes.NewReader(input)
	var output bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("ffmpeg is not installed")
		}
		return nil, errors.New("ffmpeg could not decode voicemail: " + stderr.String())
	}
	if output.Len() == 0 {
		return nil, errors.New("ffmpeg returned empty voicemail audio")
	}
	return output.Bytes(), nil
}
