package services

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	_ "embed"

	pigo "github.com/esimov/pigo/core"
)

//go:embed assets/facefinder
var cascadeData []byte

// VideoService handles video persistence, audio extraction, and face detection.
type VideoService struct {
	storagePath string
	voiceSvc    *VoiceService
	classifier  *pigo.Pigo
}

func NewVideoService(storagePath string, voiceSvc *VoiceService) (*VideoService, error) {
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("video service: create storage dir: %w", err)
	}

	classifier, err := pigo.NewPigo().Unpack(cascadeData)
	if err != nil {
		return nil, fmt.Errorf("video service: load face cascade: %w", err)
	}

	return &VideoService{
		storagePath: storagePath,
		voiceSvc:    voiceSvc,
		classifier:  classifier,
	}, nil
}

// SaveVideo writes the video bytes to disk using the session ID as the filename
// base and returns the absolute file path.
func (v *VideoService) SaveVideo(sessionID, ext string, data []byte) (string, error) {
	path := filepath.Join(v.storagePath, sessionID+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("save video: %w", err)
	}
	return path, nil
}

// ExtractAudioAndTranscribe uses ffmpeg to strip the audio track from the
// video as a 16 kHz mono WAV, then sends it to ElevenLabs STT.
func (v *VideoService) ExtractAudioAndTranscribe(ctx context.Context, videoPath string) (string, error) {
	tmp, err := os.CreateTemp("", "audio-*.wav")
	if err != nil {
		return "", fmt.Errorf("extract audio: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", videoPath,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		tmp.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg extract audio: %w: %s", err, out)
	}

	audioBytes, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("extract audio: read wav: %w", err)
	}

	return v.voiceSvc.Transcribe(ctx, audioBytes, "audio.wav")
}

// DetectMultipleFaces samples frames from the video (1 per 2 seconds, up to 20)
// and returns whether more than one face was ever visible, plus the peak count.
func (v *VideoService) DetectMultipleFaces(ctx context.Context, videoPath string) (bool, int, error) {
	tmpDir, err := os.MkdirTemp("", "frames-*")
	if err != nil {
		return false, 0, fmt.Errorf("detect faces: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", videoPath,
		"-vf", "fps=0.5",
		"-vframes", "20",
		filepath.Join(tmpDir, "frame-%03d.jpg"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, 0, fmt.Errorf("ffmpeg extract frames: %w: %s", err, out)
	}

	frames, _ := filepath.Glob(filepath.Join(tmpDir, "frame-*.jpg"))

	maxFaces := 0
	for _, frame := range frames {
		n, err := v.countFacesInImage(frame)
		if err != nil {
			continue
		}
		if n > maxFaces {
			maxFaces = n
		}
	}
	return maxFaces > 1, maxFaces, nil
}

func (v *VideoService) countFacesInImage(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, err
	}

	b := img.Bounds()
	cols, rows := b.Dx(), b.Dy()
	pixels := toGrayscale(img, b)

	cParams := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     cols,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	dets := v.classifier.RunCascade(cParams, 0.0)
	dets = v.classifier.ClusterDetections(dets, 0.2)

	count := 0
	for _, d := range dets {
		if d.Q >= 5.0 {
			count++
		}
	}
	return count, nil
}

func toGrayscale(img image.Image, b image.Rectangle) []uint8 {
	pixels := make([]uint8, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
			pixels[(y-b.Min.Y)*b.Dx()+(x-b.Min.X)] = uint8(lum)
		}
	}
	return pixels
}
