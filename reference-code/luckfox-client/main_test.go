package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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

func TestWakeVoiceCalibration(t *testing.T) {
	threshold := os.Getenv("WAKE_CALIBRATION_THRESHOLD")
	if threshold == "" {
		t.Skip("set WAKE_CALIBRATION_THRESHOLD for the private MIC1 recording")
	}
	phrase := env("WAKE_CALIBRATION_WORD", wakeWord)
	audio, err := os.ReadFile(env("WAKE_CALIBRATION_AUDIO", "/tmp/wake-voice-calibration.raw"))
	if err != nil {
		t.Fatal(err)
	}
	var filter microphoneDCBlocker
	for i := 0; i+1 < len(audio); i += 2 {
		value := int16(binary.LittleEndian.Uint16(audio[i:]))
		binary.LittleEndian.PutUint16(audio[i:], uint16(filter.Filter(value)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	decoder := os.Getenv("WAKE_CALIBRATION_BINARY")
	if decoder == "" {
		decoder = "/opt/luckfox-wakeword/luckfox-wakeword"
	}
	cmd := exec.CommandContext(ctx, decoder,
		"-hmm", "/opt/luckfox-wakeword/en-us/en-us", "-dict", "/opt/luckfox-wakeword/en-us/cmudict-en-us.dict",
		"-samprate", "16000", "-keyphrase", phrase, "-kws_threshold", threshold, "live", "-")
	cmd.Stdin = bytes.NewReader(audio)
	cmd.Stderr = os.Stderr
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		var result struct {
			Transcript string `json:"t"`
		}
		if json.Unmarshal(scanner.Bytes(), &result) == nil && result.Transcript == phrase {
			t.Logf("private MIC1 sample matched %q at threshold %s", phrase, threshold)
			return
		}
	}
	t.Fatalf("private MIC1 sample did not match %q at threshold %s", phrase, threshold)
}

func TestSingleEdisonWakeDoesNotRepeatInQuiet(t *testing.T) {
	threshold := os.Getenv("EDISON_ONCE_THRESHOLD")
	if threshold == "" {
		t.Skip("set EDISON_ONCE_THRESHOLD for the one-utterance MIC1 calibration")
	}
	audio, err := os.ReadFile("/tmp/edison-once-calibration.raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) < 5*16000*2 {
		t.Fatal("calibration must include at least five seconds of quiet at the end")
	}
	for _, test := range []struct {
		name    string
		audio   []byte
		matches int
	}{
		{"one wake then quiet", audio, 1},
		{"quiet tail only", audio[len(audio)-5*16000*2:], 0},
		{"second wake after quiet", append(append([]byte(nil), audio...), audio...), 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			audio := append([]byte(nil), test.audio...)
			var filter microphoneDCBlocker
			for i := 0; i+1 < len(audio); i += 2 {
				value := int16(binary.LittleEndian.Uint16(audio[i:]))
				binary.LittleEndian.PutUint16(audio[i:], uint16(filter.Filter(value)))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/tmp/luckfox-wakeword", "-hmm", "/opt/luckfox-wakeword/en-us/en-us",
				"-dict", "/opt/luckfox-wakeword/en-us/cmudict-en-us.dict", "-samprate", "16000",
				"-keyphrase", "edison", "-kws_threshold", threshold, "live", "-")
			cmd.Stdin = bytes.NewReader(audio)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("decoder calibration failed: %v: %s", err, output)
			}
			matches := 0
			for _, line := range bytes.Split(output, []byte{'\n'}) {
				var result struct {
					Transcript string `json:"t"`
				}
				if json.Unmarshal(line, &result) == nil && result.Transcript == "edison" {
					matches++
				}
			}
			if matches != test.matches {
				t.Fatalf("expected %d wake events; got %d at threshold %s: %s", test.matches, matches, threshold, output)
			}
			t.Logf("threshold %s: %s", threshold, output)
		})
	}
}

func TestStreamingKeywordEmitsBeforeInputCloses(t *testing.T) {
	binary := os.Getenv("WAKE_TEST_BINARY")
	if binary == "" {
		t.Skip("set WAKE_TEST_BINARY for the native PocketSphinx runtime test")
	}
	audio, err := os.ReadFile("/tmp/goforward.raw")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-hmm", "/opt/luckfox-wakeword/en-us/en-us",
		"-dict", "/opt/luckfox-wakeword/en-us/cmudict-en-us.dict", "-samprate", "16000",
		"-keyphrase", "go forward", "-kws_threshold", "1e-30", "live", "-")
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
		// Deliberately keep stdin open: detection must not wait for EOF.
	}()
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != `{"ready":true}` {
		t.Fatal("native keyword detector did not initialize")
	}
	if !scanner.Scan() || scanner.Text() != `{"t":"go forward"}` {
		t.Fatalf("keyword was not emitted during live input: %q (timeout=%v)", scanner.Text(), ctx.Err())
	}
}

type testPCMBuffer struct {
	bytes.Buffer
}

func (b *testPCMBuffer) Close() error { return nil }

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
	wake := newWakewordController(context.Background(), nil, capture,
		config{WakeWord: "edison", WakeBinary: binary, WakeModel: resources, WakeDictionary: resources}, nil)
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
