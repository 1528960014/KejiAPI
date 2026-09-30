package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ComposeTimeout bounds the whole MP4 composition for one drama.
const ComposeTimeout = 15 * time.Minute

const (
	// videoW/H is the fixed canvas for all clips so the concat step can
	// stream-copy without re-encoding.
	videoW = 1280
	videoH = 720
	// defaultClipDuration is used for shots without a voice track.
	defaultClipDuration = 4.0
	// maxClipDuration caps a single shot (TTS is one sentence per shot).
	maxClipDuration = 30.0
)

// Composer renders a drama's shots into one MP4 using the ffmpeg/ffprobe
// binaries on the server host (required dependency; CJK fonts recommended
// for subtitle rendering).
type Composer struct {
	mediaDir string
}

// NewComposer builds a Composer that writes final videos under
// <mediaDir>/dramas/.
func NewComposer(mediaDir string) *Composer {
	return &Composer{mediaDir: mediaDir}
}

// Available reports whether ffmpeg and ffprobe can be found on PATH.
func (c *Composer) Available() bool {
	_, err1 := exec.LookPath("ffmpeg")
	_, err2 := exec.LookPath("ffprobe")
	return err1 == nil && err2 == nil
}

// VideoPath returns the on-disk location of a drama's final video.
func (c *Composer) VideoPath(dramaUUID string) string {
	return filepath.Join(c.mediaDir, "dramas", dramaUUID+".mp4")
}

// Compose downloads each shot's image (and voice), renders one encoded clip
// per shot (fixed 1280x720@30, h264+aac, dialogue burned in as subtitle),
// and concatenates them into outPath. All intermediate files live in a temp
// directory that is removed when done.
func (c *Composer) Compose(ctx context.Context, shots []Shot, outPath string) error {
	if len(shots) == 0 {
		return fmt.Errorf("no shots to compose")
	}
	tmp, err := os.MkdirTemp("", "kejiapi-drama-*")
	if err != nil {
		return fmt.Errorf("make temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	listPath := filepath.Join(tmp, "list.txt")
	var list strings.Builder
	client := &http.Client{Timeout: 2 * time.Minute}

	for i, shot := range shots {
		if shot.ImageURL == "" {
			return fmt.Errorf("shot %d has no image url", shot.ShotNo)
		}
		imgPath := filepath.Join(tmp, fmt.Sprintf("img%d", i))
		if err := download(ctx, client, shot.ImageURL, imgPath); err != nil {
			return fmt.Errorf("shot %d image: %w", shot.ShotNo, err)
		}

		audioPath := ""
		var audioDur float64
		if shot.AudioURL != "" {
			audioPath = filepath.Join(tmp, fmt.Sprintf("audio%d", i))
			if err := download(ctx, client, shot.AudioURL, audioPath); err != nil {
				return fmt.Errorf("shot %d audio: %w", shot.ShotNo, err)
			}
			audioDur = probeDuration(ctx, audioPath)
		}
		dur := clipDuration(audioDur)

		sub := firstNonEmpty(shot.Dialogue, shot.Scene)
		srtName := fmt.Sprintf("sub%d.srt", i)
		if err := os.WriteFile(filepath.Join(tmp, srtName), []byte(srtFor(sub, dur)), 0o644); err != nil {
			return fmt.Errorf("write srt shot %d: %w", shot.ShotNo, err)
		}

		clipPath := filepath.Join(tmp, fmt.Sprintf("clip%d.mp4", i))
		cmd := exec.CommandContext(ctx, "ffmpeg", clipArgs(imgPath, srtName, clipPath, dur, audioPath)...)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("shot %d render: %v: %s", shot.ShotNo, err, tail(string(out), 300))
		}
		list.WriteString("file 'clip" + strconv.Itoa(i) + ".mp4'\n")
	}

	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create video dir: %w", err)
	}
	final := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", "list.txt", "-c", "copy", outPath)
	final.Dir = tmp
	if out, err := final.CombinedOutput(); err != nil {
		return fmt.Errorf("concat: %v: %s", err, tail(string(out), 300))
	}
	return nil
}

// clipArgs builds one ffmpeg invocation that renders a single shot: the
// image fills a 1280x720 canvas (letterboxed), the dialogue is burned in as
// a subtitle, and the voice (or silence) sets the clip length.
func clipArgs(imgPath, srtName, clipPath string, dur float64, audioPath string) []string {
	args := []string{"-y", "-loop", "1", "-i", imgPath}
	if audioPath != "" {
		args = append(args, "-i", audioPath)
	} else {
		args = append(args, "-f", "lavfi", "-t", fmt.Sprintf("%.3f", dur), "-i", "anullsrc=r=44100:cl=stereo")
	}
	vf := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,subtitles=%s:force_style='FontName=Noto Sans CJK SC,FontSize=18,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,MarginV=24,Alignment=2'",
		videoW, videoH, videoW, videoH, srtName,
	)
	return append(args,
		"-t", fmt.Sprintf("%.3f", dur),
		"-r", "30",
		"-vf", vf,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-ar", "44100", "-ac", "2",
		clipPath,
	)
}

// clipDuration picks a shot length: voice length + 0.5s tail when a voice
// track exists, otherwise a fixed default; clamped to sane bounds.
func clipDuration(audioDur float64) float64 {
	dur := defaultClipDuration
	if audioDur > 0 {
		dur = audioDur + 0.5
	}
	if dur < 2 {
		dur = 2
	}
	if dur > maxClipDuration {
		dur = maxClipDuration
	}
	return dur
}

// srtFor renders one cue spanning the whole clip. SRT text cannot contain
// "-->" or raw line breaks inside a cue, so we collapse them.
func srtFor(text string, dur float64) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "-->", "- -")
	text = strings.TrimSpace(text)
	if text == "" {
		text = " "
	}
	return fmt.Sprintf("1\n%s --> %s\n%s\n", ts(0), ts(dur), text)
}

// ts formats seconds as an SRT timestamp.
func ts(seconds float64) string {
	total := int(seconds * 1000)
	h := total / 3600000
	m := (total % 3600000) / 60000
	s := (total % 60000) / 1000
	ms := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// probeDuration asks ffprobe for the media duration in seconds (0 on failure).
func probeDuration(ctx context.Context, path string) float64 {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return v
}

// download fetches url into path (upstream CDN results are plain https).
func download(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(res.Body, 200*1024*1024)); err != nil {
		return err
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

var dramaUUIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// IsValidDramaUUID guards the public video route against path tricks.
func IsValidDramaUUID(uuid string) bool {
	return dramaUUIDPattern.MatchString(uuid)
}