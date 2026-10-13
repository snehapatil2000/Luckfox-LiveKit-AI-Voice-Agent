package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/livekit/media-sdk"
)

func TestMicroWakeModelConfiguration(t *testing.T) {
	directory := t.TempDir()
	model := filepath.Join(directory, "alexa.tflite")
	manifest := filepath.Join(directory, "alexa.json")
	if err := os.WriteFile(model, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(testAlexaManifest), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := microWakeArguments(model, manifest, "Alexa")
	if err != nil || len(args) != 4 || args[1] != "alexa" || args[2] != "0.9" || args[3] != "5" {
		t.Fatalf("model settings not respected: args=%v error=%v", args, err)
	}
	if _, err := microWakeArguments(model, manifest, "edison"); err == nil {
		t.Fatal("an Alexa model must not be advertised as Edison")
	}
	if err := os.WriteFile(manifest, []byte(`{"type":"micro","wake_word":"Alexa","model":"alexa.tflite","version":2,"micro":{"probability_cutoff":0.9,"sliding_window_size":0,"feature_step_size":20}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := microWakeArguments(model, manifest, "alexa"); err == nil {
		t.Fatal("unsupported feature interval/window must be rejected")
	}
}

const testAlexaManifest = `{"type":"micro","wake_word":"Alexa","model":"alexa.tflite","version":2,"micro":{"probability_cutoff":0.9,"sliding_window_size":5,"feature_step_size":10}}`

func TestStreamingKeywordEmitsBeforeInputCloses(t *testing.T) {
	runner := os.Getenv("WAKE_TEST_BINARY")
	if runner == "" {
		t.Skip("set WAKE_TEST_BINARY and WAKE_TEST_AUDIO for the ARM microWakeWord test")
	}
	audio, err := os.ReadFile(os.Getenv("WAKE_TEST_AUDIO"))
	if err != nil {
		t.Fatal(err)
	}
	args, err := microWakeArguments(env("WAKEWORD_MODEL", wakeModel), env("WAKEWORD_CONFIG", wakeManifest), "alexa")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runner, args...)
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()
	go func() {
		_, _ = input.Write(audio)
		// Keep stdin open: a microphone never reaches EOF during standby.
	}()
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != `{"ready":true}` {
		t.Fatal("detector did not initialize")
	}
	var result struct {
		Word string `json:"t"`
	}
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &result) != nil || result.Word != "alexa" {
		t.Fatalf("Alexa was not emitted during live input: %q (timeout=%v)", scanner.Text(), ctx.Err())
	}
}

type testPCMBuffer struct {
	bytes.Buffer
}

func (b *testPCMBuffer) Close() error { return nil }

func TestWakeBeepUsesMonoPlaybackFormat(t *testing.T) {
	buffer := &testPCMBuffer{}
	writer := &aplayWriter{in: buffer, muted: true}
	if err := playWakeBeep(writer); err != nil {
		t.Fatal(err)
	}
	if outputChannels != 1 || buffer.Len() != 38400 {
		t.Fatalf("400 ms at 48 kHz mono must produce 38400 bytes, got channels=%d bytes=%d", outputChannels, buffer.Len())
	}
}

func TestInterruptedPlaybackDropsOldAudioButAllowsWakeBeep(t *testing.T) {
	buffer := &testPCMBuffer{}
	writer := &aplayWriter{in: buffer, muted: true}
	sample := media.PCM16Sample{100, -100}
	if err := writer.WriteSample(sample); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != 0 {
		t.Fatal("old remote audio must be discarded after wake interruption")
	}
	if err := writer.writeSample(sample, true); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != 4 {
		t.Fatal("wake acknowledgement must still play while the old answer is muted")
	}
	writer.Resume()
	if err := writer.WriteSample(sample); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != 8 {
		t.Fatal("the next AI answer must resume playback")
	}
}

func TestWakeDecoderBacklogDoesNotBlockMicrophone(t *testing.T) {
	wake := &wakewordController{frames: make(chan []byte, 2)}
	for i := 0; i < 100; i++ {
		if err := wake.feed([]byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if oldest := <-wake.frames; oldest[0] != 98 {
		t.Fatalf("queue must retain fresh audio, got %d", oldest[0])
	}
	wake.capture = newCaptureController(context.Background(), nil, "unused")
	wake.Pause()
	if len(wake.frames) != 0 {
		t.Fatal("beep/listen transition must flush queued wake audio")
	}
}

func TestWakeListenerCanRestart(t *testing.T) {
	resources := t.TempDir()
	binary := filepath.Join(resources, "detector")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '{\"ready\":true}\\n'\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	capture := newCaptureController(context.Background(), nil, "unused")
	model := filepath.Join(resources, "alexa.tflite")
	manifest := filepath.Join(resources, "alexa.json")
	if err := os.WriteFile(model, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(testAlexaManifest), 0600); err != nil {
		t.Fatal(err)
	}
	wake := newWakewordController(context.Background(), nil, capture,
		config{WakeWord: "alexa", WakeBinary: binary, WakeModel: model, WakeManifest: manifest}, nil)
	defer wake.Stop()
	for cycle := 0; cycle < 3; cycle++ {
		wake.SetQuestionPending(true)
		if err := wake.Start(); err != nil {
			t.Fatal(err)
		}
		if !wake.QuestionPending() {
			t.Fatal("restarting the local listener must not clear a pending question")
		}
		capture.mu.Lock()
		sink, cloudEnabled := capture.wakeSink, capture.cloudEnabled
		capture.mu.Unlock()
		if sink == nil || cloudEnabled {
			t.Fatal("standby must route MIC1 to the local detector only")
		}
		if err := sink(make([]byte, 640)); err != nil {
			t.Fatal(err)
		}
		originalInput := wake.input
		wake.Pause()
		capture.mu.Lock()
		sink = capture.wakeSink
		capture.mu.Unlock()
		if sink != nil {
			t.Fatal("beep must pause local wake input")
		}
		if err := wake.Start(); err != nil || wake.input != originalInput {
			t.Fatalf("resuming wake listening must reuse the loaded model: %v", err)
		}
		wake.Stop()
		capture.mu.Lock()
		sink = capture.wakeSink
		capture.mu.Unlock()
		if sink != nil {
			t.Fatal("beep/playback must disconnect the wake detector")
		}
	}
}

func TestMicrophoneDCBlocker(t *testing.T) {
	var quiet microphoneDCBlocker
	for i := 0; i < 16000; i++ {
		if sample := quiet.Filter(-16000); sample != 0 {
			t.Fatalf("constant microphone offset produced audio: %d", sample)
		}
	}
	var speech microphoneDCBlocker
	var sum, squares float64
	for i := 0; i < 16000; i++ {
		value := int16(-16000 + 1000*math.Sin(2*math.Pi*440*float64(i)/16000))
		filtered := float64(speech.Filter(value))
		if i >= 8000 {
			sum += filtered
			squares += filtered * filtered
		}
	}
	mean := sum / 8000
	rms := math.Sqrt(squares/8000 - mean*mean)
	if math.Abs(mean) > 5 || rms < 650 || rms > 750 {
		t.Fatalf("voice must remain while offset is removed: mean=%.1f rms=%.1f", mean, rms)
	}
}

func TestCaptureReadiness(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		if err := (&captureController{}).WaitReady(time.Millisecond); err == nil {
			t.Fatal("capture must be started before arming the question")
		}
	})
	t.Run("first frame received", func(t *testing.T) {
		ready := make(chan struct{})
		close(ready)
		capture := &captureController{ready: ready, done: make(chan struct{})}
		if err := capture.WaitReady(time.Millisecond); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("recorder exits without audio", func(t *testing.T) {
		done := make(chan struct{})
		close(done)
		capture := &captureController{ready: make(chan struct{}), done: done}
		if err := capture.WaitReady(time.Second); err == nil {
			t.Fatal("recorder exit must not arm an empty question")
		}
	})
	t.Run("recorder stalls", func(t *testing.T) {
		capture := &captureController{ready: make(chan struct{}), done: make(chan struct{})}
		if err := capture.WaitReady(time.Millisecond); err == nil {
			t.Fatal("stalled recorder must time out")
		}
	})
}

func TestAllowedAutomationRequests(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		allowed bool
	}{
		{"GET", "/api/v1/points", true},
		{"GET", "/api/v1/rooms", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/on", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/set", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/up", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/down", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/set", true},
		{"POST", "/api/v1/scenes/7/activate", true},
		{"POST", "/api/v1/moods/9/activate", true},
		{"DELETE", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11", false},
		{"POST", "/api/v1/users/1/delete", false},
		{"POST", "/api/v1/points/../../users/on", false},
		{"GET", "/api/v1/auth/tokens", false},
	}
	for _, test := range tests {
		if got := isAllowedAutomationRequest(test.method, test.path); got != test.allowed {
			t.Errorf("isAllowedAutomationRequest(%q, %q) = %v, want %v", test.method, test.path, got, test.allowed)
		}
	}
}

func TestAutomationBridgeLogsInAndRetriesExpiredToken(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"access_token": "fresh-token"}})
		case "/api/v1/points":
			if r.Header.Get("Authorization") == "Bearer expired-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer fresh-token" {
				t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"points":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	bridge := &automationBridge{
		resolver: staticGatewayResolver(server.URL),
		token:    "expired-token",
		username: "voice_agent",
		password: "secret",
		client:   server.Client(),
	}
	status, _, err := bridge.request(geckoLinkRequest{Method: http.MethodGet, Path: "/api/v1/points"})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || loginCount != 1 {
		t.Fatalf("status=%d loginCount=%d, want 200 and 1", status, loginCount)
	}
}
