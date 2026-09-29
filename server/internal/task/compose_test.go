package task

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestClipDuration(t *testing.T) {
	if got := clipDuration(0); got != defaultClipDuration {
		t.Errorf("clipDuration(no audio) = %v, want %v", got, defaultClipDuration)
	}
	if got := clipDuration(6.2); got != 6.7 {
		t.Errorf("clipDuration(6.2s audio) = %v, want 6.7 (audio + 0.5 tail)", got)
	}
	if got := clipDuration(0.1); got != 2 {
		t.Errorf("clipDuration(0.1s) = %v, want floor 2", got)
	}
	if got := clipDuration(120); got != maxClipDuration {
		t.Errorf("clipDuration(120s) = %v, want cap %v", got, maxClipDuration)
	}
}

func TestSrtFor(t *testing.T) {
	got := srtFor("雨夜，他撑伞。", 4)
	want := "1\n00:00:00,000 --> 00:00:04,000\n雨夜，他撑伞。\n"
	if got != want {
		t.Errorf("srtFor = %q, want %q", got, want)
	}
	// line breaks collapse, "-->" is neutralized
	got = srtFor("line1\nline2 --> x", 2.5)
	if strings.Contains(got, "\nline2") || strings.Contains(got, "--> x") {
		t.Errorf("srtFor did not sanitize: %q", got)
	}
	if !strings.Contains(got, "line1 line2 - - x") {
		t.Errorf("srtFor = %q, want collapsed text", got)
	}
	// empty text must still yield a valid cue
	got = srtFor("  ", 3)
	if strings.Count(got, "\n") != 3 {
		t.Errorf("srtFor(empty) = %q, want cue with blank text", got)
	}
}

func TestTS(t *testing.T) {
	if got := ts(3661.5); got != "01:01:01,500" {
		t.Errorf("ts(3661.5) = %q, want 01:01:01,500", got)
	}
	if got := ts(0); got != "00:00:00,000" {
		t.Errorf("ts(0) = %q", got)
	}
}

func TestClipArgs(t *testing.T) {
	// with voice
	args := clipArgs("img0", "sub0.srt", "clip0.mp4", 6.7, "audio0")
	s := strings.Join(args, " ")
	for _, want := range []string{
		"-loop 1 -i img0", "-i audio0", "-t 6.700", "-r 30",
		"scale=1280:720:force_original_aspect_ratio=decrease",
		"subtitles=sub0.srt", "libx264", "yuv420p", "aac", "clip0.mp4",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("clipArgs(with audio) missing %q in %s", want, s)
		}
	}
	// without voice: silence source instead
	args = clipArgs("img0", "sub0.srt", "clip0.mp4", 4, "")
	s = strings.Join(args, " ")
	if !strings.Contains(s, "anullsrc=r=44100:cl=stereo") || strings.Contains(s, "-i audio0") {
		t.Errorf("clipArgs(no audio) = %s", s)
	}
}

func TestIsValidDramaUUID(t *testing.T) {
	if !IsValidDramaUUID("0123456789abcdef0123456789abcdef") {
		t.Error("valid uuid rejected")
	}
	for _, bad := range []string{
		"0123456789abcdef0123456789abcde",  // 31 chars
		"0123456789abcdef0123456789abcdef0", // 33 chars
		"../../etc/passwd",
		"ABCDEF0123456789ABCDEF0123456789", // uppercase rejected (uuids are lowercase hex)
	} {
		if IsValidDramaUUID(bad) {
			t.Errorf("IsValidDramaUUID(%q) = true, want false", bad)
		}
	}
}

// TestComposeEndToEnd renders a real two-shot drama with local test assets
// (ffmpeg-generated image + sine tone, served over httptest). Skipped when
// ffmpeg/ffprobe are not installed.
func TestComposeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed; skipping end-to-end compose")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed; skipping end-to-end compose")
	}
	ctx := context.Background()

	dir := t.TempDir()
	img := filepath.Join(dir, "img.png")
	if out, err := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", "testsrc=duration=1:size=640x480:rate=1", "-frames:v", "1", img).CombinedOutput(); err != nil {
		t.Fatalf("make test image: %v: %s", err, out)
	}
	audio := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=2", audio).CombinedOutput(); err != nil {
		t.Fatalf("make test audio: %v: %s", err, out)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "dramas", "0123456789abcdef0123456789abcdef.mp4")
	composer := NewComposer(filepath.Dir(out))
	shots := []Shot{
		{ShotNo: 1, Scene: "雨夜", Dialogue: "你终于来了。", ImagePrompt: "rainy night", Status: "succeeded",
			ImageURL: srv.URL + "/img.png", AudioURL: srv.URL + "/tone.wav"},
		{ShotNo: 2, Scene: "黎明", Dialogue: "", ImagePrompt: "dawn", Status: "succeeded",
			ImageURL: srv.URL + "/img.png"},
	}
	if err := composer.Compose(ctx, shots, out); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if info.Size() < 10_000 {
		t.Errorf("output suspiciously small: %d bytes", info.Size())
	}
	// expected total: shot1 2s audio + 0.5 tail + shot2 default 4s = 6.5s
	raw, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", out).Output()
	if err != nil {
		t.Fatalf("ffprobe output: %v", err)
	}
	dur, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		t.Fatalf("parse duration: %v", err)
	}
	if dur < 6.0 || dur > 7.5 {
		t.Errorf("composed duration = %.2fs, want ~6.5s", dur)
	}
}