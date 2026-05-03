package camera

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Camera struct {
	rtspURL string
}

func New(rtspURL string) *Camera {
	return &Camera{rtspURL: rtspURL}
}

// Grab pulls a single JPEG frame from the RTSP stream by shelling out to ffmpeg.
// Requires ffmpeg on PATH.
func (c *Camera) Grab(ctx context.Context) ([]byte, error) {
	args := []string{
		"-rtsp_transport", "tcp",
		"-i", c.rtspURL,
		"-frames:v", "1",
		"-f", "image2",
		"-vcodec", "mjpeg",
		"-loglevel", "error",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w (%s)", err, stderr.String())
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg returned empty frame: %s", stderr.String())
	}
	return stdout.Bytes(), nil
}
