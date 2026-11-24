package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/sandbox"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxVoiceBytes = 44 + 90*16000*2

var speechSlots = make(chan struct{}, 1)

func localSpeech() (string, string, bool) {
	name, err := exec.LookPath("whisper-cli")
	model := os.Getenv("HEALTHCTL_WHISPER_MODEL")
	if model == "" {
		model = "/usr/share/healthctl/whisper.bin"
	}
	info, statErr := os.Stat(model)
	return name, model, err == nil && statErr == nil && info.Mode().IsRegular() && info.Size() > 0
}

func (s *Server) voice(w http.ResponseWriter, r *http.Request, _ int) error {
	cli, model, local := localSpeech()
	_, creds := s.Models.Status()
	if r.Method == "GET" {
		emit(w, 200, object{"local": local, "openai": creds["openai"], "chatgptPlan": false})
		return nil
	}
	mode := r.URL.Query().Get("destination")
	if mode != "local" && mode != "openai" {
		return bad("Choose a transcription destination.")
	}
	if mode == "openai" && (!creds["openai"] || r.Header.Get("X-Audio-Consent") != "openai-api") {
		return bad("Confirm sending this recording to the configured OpenAI API.")
	}
	if mode == "local" && !local {
		return problem{503, "Local speech recognition is unavailable. Choose another input method."}
	}
	if r.Header.Get("Content-Type") != "audio/wav" {
		return problem{415, "Record a mono PCM WAV clip."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVoiceBytes)
	wav, err := io.ReadAll(r.Body)
	if err != nil || !voiceWAV(wav) {
		return bad("Choose a 16 kHz mono recording of up to 90 seconds.")
	}
	select {
	case speechSlots <- struct{}{}:
		defer func() { <-speechSlots }()
	default:
		return problem{429, "Another recording is being transcribed. Retry in a moment."}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var transcript string
	if mode == "openai" {
		transcript, err = s.Models.Transcribe(ctx, wav)
		if err != nil {
			return problem{422, err.Error()}
		}
	} else {
		dir, err := os.MkdirTemp("", "healthctl-voice-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		input := filepath.Join(dir, "recording.wav")
		out := filepath.Join(dir, "draft")
		if err = os.WriteFile(input, wav, 0600); err != nil {
			return err
		}
		// The speech reader gets no secrets and, where possible, no network.
		cmd, cancel := sandbox.Command(ctx, sandbox.SpeechLimits, cli, "-m", model, "-f", input, "-l", "en", "-t", "2", "-nt", "-otxt", "-of", out)
		defer cancel()
		if cmd.Run() != nil {
			return problem{422, "Local transcription failed. The recording is available to retry or transcribe another way."}
		}
		f, err := os.Open(out + ".txt")
		if err != nil {
			return problem{422, "No transcript was produced."}
		}
		defer f.Close()
		result, err := io.ReadAll(io.LimitReader(f, 8193))
		if err != nil || len(result) > 8192 {
			return bad("The transcript exceeds the review limit.")
		}
		transcript = strings.TrimSpace(string(result))
		if transcript == "" {
			return bad("No speech was recognized.")
		}
	}
	emit(w, 200, s.draftResponse(transcript, mode))
	return nil
}

// draftResponse screens a dictation draft before the user reviews it, so an
// urgent report is visible even if the words are never saved.
func (s *Server) draftResponse(transcript, mode string) object {
	out := object{"draft": transcript, "destination": mode}
	if res, ok := clinical.ScreenText(transcript, s.Emergency); ok {
		out["warning"] = res
	}
	return out
}

// The browser produces one canonical PCM header. Reject alternate containers
// and declared lengths that disagree before passing media to either processor.
func voiceWAV(b []byte) bool {
	if len(b) < 46 || len(b) > maxVoiceBytes || (len(b)-44)%2 != 0 {
		return false
	}
	n := binary.LittleEndian
	return bytes.Equal(b[:4], []byte("RIFF")) && int(n.Uint32(b[4:])) == len(b)-8 && string(b[8:16]) == "WAVEfmt " && n.Uint32(b[16:]) == 16 && n.Uint16(b[20:]) == 1 && n.Uint16(b[22:]) == 1 && n.Uint32(b[24:]) == 16000 && n.Uint32(b[28:]) == 32000 && n.Uint16(b[32:]) == 2 && n.Uint16(b[34:]) == 16 && string(b[36:40]) == "data" && int(n.Uint32(b[40:])) == len(b)-44
}
