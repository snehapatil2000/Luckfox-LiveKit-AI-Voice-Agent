package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	media "github.com/livekit/media-sdk"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	lkmedia "github.com/livekit/server-sdk-go/v2/pkg/media"
	"github.com/pion/webrtc/v4"
)

const (
	sampleRate = 48000
	// Use the same native mono capture format as the working wake listener.
	inputChannels = 1
	// Agent speech is mono; stereo decoding wastes work needed by the wake engine.
	outputChannels     = 1
	frameMillis        = 20
	controlTopic       = "luckfox-control"
	activationEvent    = "arm_question"
	interruptEvent     = "interrupt_answer"
	micSleepEvent      = "mic_sleep"
	assistantSpeaking  = "assistant_speaking"
	assistantListening = "assistant_listening"
	geckoRequestEvent  = "geckolink_request"
	geckoResponseEvent = "geckolink_response"
	wakeSampleRate     = 16000
	wakeWord           = "alexa"
	wakeBinary         = "/opt/luckfox-microwakeword/run-microwakeword.sh"
	wakeModel          = "/opt/luckfox-microwakeword/alexa.tflite"
	wakeManifest       = "/opt/luckfox-microwakeword/alexa.json"
	workLEDTrigger     = "/sys/class/leds/work/trigger"
	workLEDBrightness  = "/sys/class/leds/work/brightness"
	wakeBeepDuration   = 400 * time.Millisecond
	// aplay buffers audio, so wait for the audible confirmation to finish
	// before starting cloud capture and its 1.5-second question timer.
	wakeBeepSettle = 600 * time.Millisecond
)

type config struct {
	URL, APIKey, APISecret, Token        string
	Room, Identity, AgentName            string
	CaptureDevice, PlaybackDevice        string
	WakeCaptureDevice, WakeWord          string
	WakeBinary, WakeModel, WakeManifest  string
	GeckoLinkToken                       string
	GeckoLinkUsername, GeckoLinkPassword string
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadConfig() (config, error) {
	c := config{
		URL:       os.Getenv("LIVEKIT_URL"),
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		Token:     os.Getenv("LIVEKIT_TOKEN"),
		Room:      env("LIVEKIT_ROOM", "luckfox-voice"),
		Identity:  env("LIVEKIT_IDENTITY", "luckfox-board"),
		AgentName: env("LIVEKIT_AGENT_NAME", "my-agent"),
		// plughw performs the board-specific format conversion and supports
		// simultaneous MIC1 capture and speaker playback more reliably than
		// the raw hw device.
		CaptureDevice:     env("CAPTURE_DEVICE", "plughw:0,0"),
		PlaybackDevice:    env("PLAYBACK_DEVICE", "plughw:0,0"),
		WakeCaptureDevice: env("WAKE_CAPTURE_DEVICE", "plughw:0,0"),
		WakeWord:          strings.ToLower(env("WAKE_WORD", wakeWord)),
		WakeBinary:        env("WAKEWORD_BINARY", wakeBinary),
		WakeModel:         env("WAKEWORD_MODEL", wakeModel),
		WakeManifest:      env("WAKEWORD_CONFIG", wakeManifest),
		GeckoLinkToken:    os.Getenv("GECKOLINK_TOKEN"),
		GeckoLinkUsername: os.Getenv("GECKOLINK_USERNAME"),
		GeckoLinkPassword: os.Getenv("GECKOLINK_PASSWORD"),
	}
	if c.URL == "" {
		return c, errors.New("LIVEKIT_URL is required")
	}
	if c.Token == "" && (c.APIKey == "" || c.APISecret == "") {
		return c, errors.New("set LIVEKIT_TOKEN or LIVEKIT_API_KEY and LIVEKIT_API_SECRET")
	}
	return c, nil
}

type aplayWriter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser
	closed bool
	muted  bool
	device string
}

func newAplayWriter(device string) (*aplayWriter, error) {
	w := &aplayWriter{device: device}
	if err := w.startLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *aplayWriter) startLocked() error {
	cmd := exec.Command("aplay", "-q", "-D", w.device, "-t", "raw", "-f", "S16_LE", "-r", fmt.Sprint(sampleRate), "-c", fmt.Sprint(outputChannels), "--buffer-time=100000", "--period-time=20000")
	// The remote agent track is silent between replies. ALSA reports each
	// resulting empty playback interval as an underrun, which would otherwise
	// fill the device log even though later speech continues normally.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	w.cmd, w.in = cmd, in
	return nil
}

func (w *aplayWriter) WriteSample(sample media.PCM16Sample) error {
	return w.writeSample(sample, false)
}

func (w *aplayWriter) writeSample(sample media.PCM16Sample, acknowledgement bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return io.ErrClosedPipe
	}
	if w.muted && !acknowledgement {
		return nil
	}
	if w.in == nil {
		if err := w.startLocked(); err != nil {
			return err
		}
	}
	buf := make([]byte, len(sample)*2)
	for i, value := range sample {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(value))
	}
	_, err := w.in.Write(buf)
	return err
}

func (w *aplayWriter) Interrupt() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.muted = true
	// Killing aplay also drops the old ALSA/pipe buffers. Further packets from
	// the interrupted remote stream are discarded until the new answer starts.
	if w.cmd != nil {
		_ = w.cmd.Process.Kill()
	}
	if w.in != nil {
		_ = w.in.Close()
	}
	if w.cmd != nil {
		_ = w.cmd.Wait()
	}
	w.cmd, w.in = nil, nil
}

func (w *aplayWriter) Resume() {
	w.mu.Lock()
	w.muted = false
	w.mu.Unlock()
}

func (w *aplayWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	var err error
	if w.in != nil {
		err = w.in.Close()
	}
	cmd := w.cmd
	w.mu.Unlock()
	if cmd != nil {
		_ = cmd.Wait()
	}
	return err
}

func ensureAgent(c config) error {
	if c.Token != "" || c.AgentName == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := lksdk.NewAgentDispatchServiceClient(c.URL, c.APIKey, c.APISecret)
	result, err := client.ListDispatch(ctx, &livekit.ListAgentDispatchRequest{Room: c.Room})
	if err != nil {
		return fmt.Errorf("list agent dispatches: %w", err)
	}
	for _, dispatch := range result.AgentDispatches {
		if dispatch.AgentName == c.AgentName {
			fmt.Printf("Agent %s already dispatched\n", c.AgentName)
			return nil
		}
	}
	_, err = client.CreateDispatch(ctx, &livekit.CreateAgentDispatchRequest{Room: c.Room, AgentName: c.AgentName})
	if err != nil {
		return fmt.Errorf("dispatch agent: %w", err)
	}
	fmt.Printf("Dispatched agent %s\n", c.AgentName)
	return nil
}

var (
	pointActionPath = regexp.MustCompile(`^/api/v1/points/[0-9A-Fa-f:]+/(on|off|toggle|set|fan/toggle|fan/speed/(up|down|set))$`)
	groupActionPath = regexp.MustCompile(`^/api/v1/(scenes|moods)/[0-9]+/activate$`)
)

type geckoLinkRequest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Method    string          `json:"method"`
	Path      string          `json:"path"`
	Body      json.RawMessage `json:"body"`
}

type geckoLinkResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Status    int    `json:"status"`
	Body      any    `json:"body,omitempty"`
	Error     string `json:"error,omitempty"`
}

type automationBridge struct {
	resolver    *gatewayResolver
	token       string
	username    string
	password    string
	authMu      sync.Mutex
	client      *http.Client
	participant *lksdk.LocalParticipant
}

func newAutomationBridge(c config, participant *lksdk.LocalParticipant) *automationBridge {
	return &automationBridge{
		resolver:    newGatewayResolver(),
		token:       c.GeckoLinkToken,
		username:    c.GeckoLinkUsername,
		password:    c.GeckoLinkPassword,
		client:      &http.Client{Timeout: 12 * time.Second},
		participant: participant,
	}
}

func (b *automationBridge) accessToken(baseURL string, forceLogin bool) (string, error) {
	b.authMu.Lock()
	defer b.authMu.Unlock()
	if !forceLogin && b.token != "" {
		return b.token, nil
	}
	if b.username == "" || b.password == "" {
		return "", errors.New("GECKOLINK_TOKEN or GECKOLINK_USERNAME and GECKOLINK_PASSWORD are required")
	}
	payload, _ := json.Marshal(map[string]any{
		"identifier": b.username,
		"password":   b.password,
		"rememberMe": true,
	})
	response, err := b.client.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("GeckoLink login failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("cannot read GeckoLink login response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("GeckoLink login returned HTTP %d", response.StatusCode)
	}
	var result struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
		Data        struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("invalid GeckoLink login response: %w", err)
	}
	if result.AccessToken == "" {
		result.AccessToken = result.Token
	}
	if result.AccessToken == "" {
		result.AccessToken = result.Data.AccessToken
	}
	if result.AccessToken == "" {
		return "", errors.New("GeckoLink login response has no access token")
	}
	b.token = result.AccessToken
	return b.token, nil
}

func (b *automationBridge) request(request geckoLinkRequest) (int, []byte, error) {
	for addressAttempt := 0; addressAttempt < 2; addressAttempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		baseURL, err := b.resolver.URL(ctx, addressAttempt > 0)
		cancel()
		if err != nil {
			return 0, nil, err
		}
		for authAttempt := 0; authAttempt < 2; authAttempt++ {
			token, tokenErr := b.accessToken(baseURL, authAttempt > 0)
			if tokenErr != nil {
				return 0, nil, tokenErr
			}
			var body io.Reader
			if len(request.Body) > 0 && string(request.Body) != "null" {
				body = bytes.NewReader(request.Body)
			}
			httpRequest, requestErr := http.NewRequest(strings.ToUpper(request.Method), baseURL+request.Path, body)
			if requestErr != nil {
				return 0, nil, requestErr
			}
			httpRequest.Header.Set("Authorization", "Bearer "+token)
			httpRequest.Header.Set("Accept", "application/json")
			if body != nil {
				httpRequest.Header.Set("Content-Type", "application/json")
			}
			httpResponse, requestErr := b.client.Do(httpRequest)
			if requestErr != nil {
				if addressAttempt == 0 {
					break
				}
				return 0, nil, fmt.Errorf("cannot reach GeckoLink gateway: %w", requestErr)
			}
			data, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
			httpResponse.Body.Close()
			if readErr != nil {
				return 0, nil, fmt.Errorf("cannot read GeckoLink response: %w", readErr)
			}
			if httpResponse.StatusCode != http.StatusUnauthorized || b.username == "" || b.password == "" {
				return httpResponse.StatusCode, data, nil
			}
		}
	}
	return 0, nil, errors.New("cannot reach GeckoLink gateway after refreshing its IP")
}

func isAllowedAutomationRequest(method, path string) bool {
	method = strings.ToUpper(method)
	if method == http.MethodGet {
		switch path {
		case "/api/v1/points", "/api/v1/rooms", "/api/v1/scenes", "/api/v1/moods":
			return true
		}
	}
	return method == http.MethodPost && (pointActionPath.MatchString(path) || groupActionPath.MatchString(path))
}

func (b *automationBridge) publish(response geckoLinkResponse, destination string) {
	payload, err := json.Marshal(response)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GeckoLink response encoding error: %v\n", err)
		return
	}
	if err = b.participant.PublishDataPacket(
		lksdk.UserData(payload),
		lksdk.WithDataPublishTopic(controlTopic),
		lksdk.WithDataPublishReliable(true),
		lksdk.WithDataPublishDestination([]string{destination}),
	); err != nil {
		fmt.Fprintf(os.Stderr, "GeckoLink response publish error: %v\n", err)
	}
}

func (b *automationBridge) handle(payload []byte, destination string) {
	var request geckoLinkRequest
	if err := json.Unmarshal(payload, &request); err != nil || request.Type != geckoRequestEvent {
		return
	}
	response := geckoLinkResponse{Type: geckoResponseEvent, RequestID: request.RequestID}
	if request.RequestID == "" {
		return
	}
	if b.token == "" && (b.username == "" || b.password == "") {
		response.Status = http.StatusServiceUnavailable
		response.Error = "GeckoLink credentials are not configured on Luckfox"
		b.publish(response, destination)
		return
	}
	if !isAllowedAutomationRequest(request.Method, request.Path) {
		response.Status = http.StatusForbidden
		response.Error = "automation endpoint is not allowed"
		b.publish(response, destination)
		return
	}

	status, data, err := b.request(request)
	if err != nil {
		response.Status = http.StatusBadGateway
		response.Error = err.Error()
		b.publish(response, destination)
		return
	}
	response.Status = status
	if len(data) > 0 {
		if err = json.Unmarshal(data, &response.Body); err != nil {
			response.Body = map[string]any{"message": string(data)}
		}
	} else {
		response.Body = map[string]any{}
	}
	b.publish(response, destination)
	fmt.Printf("GeckoLink %s %s -> %d\n", request.Method, request.Path, response.Status)
}

type captureController struct {
	ctx          context.Context
	track        *lkmedia.PCMLocalTrack
	device       string
	errCh        chan error
	mu           sync.Mutex
	cancel       context.CancelFunc
	done         chan struct{}
	ready        chan struct{}
	cloudEnabled bool
	wakeSink     func([]byte) error
}

type microphoneDCBlocker struct {
	initialized bool
	previousIn  float64
	previousOut float64
}

func (f *microphoneDCBlocker) Filter(value int16) int16 {
	x := float64(value)
	if !f.initialized {
		f.initialized = true
		f.previousIn = x
		return 0
	}
	y := x - f.previousIn + 0.995*f.previousOut
	f.previousIn, f.previousOut = x, y
	return int16(math.Max(-32768, math.Min(32767, y)))
}

func newCaptureController(ctx context.Context, track *lkmedia.PCMLocalTrack, device string) *captureController {
	return &captureController{ctx: ctx, track: track, device: device, errCh: make(chan error, 1)}
}

// Start owns MIC1 for the session. Corrected 16 kHz mono frames go to the
// local wake detector in standby or to LiveKit during an armed question.
func (c *captureController) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(c.ctx)
	cmd := exec.CommandContext(ctx, "arecord", "-q", "-D", c.device, "-t", "raw", "-f", "S16_LE", "-r", fmt.Sprint(wakeSampleRate), "-c", fmt.Sprint(inputChannels), "--buffer-time=1000000", "--period-time=20000")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err = cmd.Start(); err != nil {
		cancel()
		return err
	}
	done := make(chan struct{})
	ready := make(chan struct{})
	c.cancel = cancel
	c.done = done
	c.ready = ready
	startedAt := time.Now()
	fmt.Printf("MIC1 ON: continuous local capture from %s\n", c.device)

	go func() {
		defer close(done)
		frameSamples := wakeSampleRate * frameMillis / 1000
		raw := make([]byte, frameSamples*inputChannels*2)
		mono := make(media.PCM16Sample, frameSamples)
		var captureErr error
		framesRead := 0
		var dcBlocker microphoneDCBlocker
		for {
			_, readErr := io.ReadFull(out, raw)
			if readErr != nil {
				if ctx.Err() == nil {
					captureErr = fmt.Errorf("read microphone: %w", readErr)
				}
				break
			}
			for i := 0; i < frameSamples; i++ {
				value := int16(binary.LittleEndian.Uint16(raw[i*2:]))
				mono[i] = dcBlocker.Filter(value)
			}
			if framesRead == 0 {
				fmt.Printf("MIC1 READY: first PCM frame after %s\n", time.Since(startedAt))
				close(ready)
			}
			framesRead++
			if framesRead == 1 || framesRead%25 == 0 {
				var sum, squares float64
				for _, value := range mono {
					v := float64(value)
					sum += v
					squares += v * v
				}
				mean := sum / float64(len(mono))
				rms := math.Sqrt(math.Max(0, squares/float64(len(mono))-mean*mean))
				fmt.Printf("MIC1 AUDIO: frames=%d format=16000Hz-mono mean=%.0f voice_rms=%.0f\n", framesRead, mean, rms)
			}

			c.mu.Lock()
			cloudEnabled := c.cloudEnabled
			wakeSink := c.wakeSink
			c.mu.Unlock()
			if wakeSink != nil && !cloudEnabled {
				pcm := make([]byte, len(mono)*2)
				for i, value := range mono {
					binary.LittleEndian.PutUint16(pcm[i*2:], uint16(value))
				}
				if err := wakeSink(pcm); err != nil && ctx.Err() == nil {
					fmt.Fprintf(os.Stderr, "wake audio input error: %v\n", err)
				}
			}
			if !cloudEnabled {
				continue
			}
			if writeErr := c.track.WriteSample(mono); writeErr != nil {
				captureErr = fmt.Errorf("publish microphone: %w", writeErr)
				break
			}
		}
		_ = cmd.Wait()
		c.mu.Lock()
		if c.done == done {
			c.cancel = nil
			c.done = nil
		}
		c.mu.Unlock()
		if captureErr != nil {
			select {
			case c.errCh <- captureErr:
			default:
			}
		}
	}()
	return nil
}

func (c *captureController) WaitReady(timeout time.Duration) error {
	c.mu.Lock()
	ready, done := c.ready, c.done
	c.mu.Unlock()
	if ready == nil || done == nil {
		return errors.New("MIC1 capture has not started")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ready:
		return nil
	case <-done:
		return errors.New("MIC1 capture stopped before its first PCM frame")
	case <-timer.C:
		return errors.New("MIC1 did not supply a PCM frame before capture timeout")
	}
}

func (c *captureController) SetCloudEnabled(enabled bool) {
	c.mu.Lock()
	changed := c.cloudEnabled != enabled
	c.cloudEnabled = enabled
	c.mu.Unlock()
	if changed {
		if enabled {
			fmt.Println("MIC1 LISTEN: publishing question audio to LiveKit")
		} else {
			fmt.Println("MIC1 STANDBY: question audio is local only")
		}
	}
}

func (c *captureController) SetWakeSink(sink func([]byte) error) {
	c.mu.Lock()
	c.wakeSink = sink
	c.mu.Unlock()
}

func (c *captureController) Stop() {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	fmt.Println("MIC1 OFF: continuous capture stopped")
}

type wakewordController struct {
	ctx                           context.Context
	participant                   *lksdk.LocalParticipant
	capture                       *captureController
	playBeep                      func()
	interruptPlayback             func()
	word, binary, model, manifest string
	errCh                         chan error
	startMu                       sync.Mutex
	mu                            sync.Mutex
	cancel                        context.CancelFunc
	done                          chan struct{}
	input                         io.WriteCloser
	triggered                     bool
	questionPending               bool
	paused                        bool
	decoderReady                  bool
	frames                        chan []byte
	droppedFrames                 int
	acceptWakeAfter               time.Time
}

func newWakewordController(ctx context.Context, participant *lksdk.LocalParticipant, capture *captureController, c config, playBeep func()) *wakewordController {
	return &wakewordController{
		ctx: ctx, participant: participant, capture: capture,
		playBeep: playBeep,
		word:     c.WakeWord, binary: c.WakeBinary, model: c.WakeModel, manifest: c.WakeManifest,
		errCh: make(chan error, 1),
	}
}

func setWorkLED(on bool) {
	_ = os.WriteFile(workLEDTrigger, []byte("none"), 0o644)
	value := "0"
	if on {
		value = "255"
	}
	if err := os.WriteFile(workLEDBrightness, []byte(value), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "work LED error: %v\n", err)
	}
}

func playWakeBeep(writer *aplayWriter) error {
	if writer == nil {
		return errors.New("no active speaker")
	}
	frames := int(sampleRate * wakeBeepDuration / time.Second)
	buf := make(media.PCM16Sample, frames*outputChannels)
	for i := 0; i < frames; i++ {
		value := int16(math.Sin(2*math.Pi*880*float64(i)/sampleRate) * 9000)
		for channel := 0; channel < outputChannels; channel++ {
			buf[i*outputChannels+channel] = value
		}
	}
	if err := writer.writeSample(buf, true); err != nil {
		return err
	}
	return nil
}

// Read the settings shipped with the model. A label cannot change the word
// recognized by a neural model, so reject mismatched model configuration.
func microWakeArguments(model, manifest, word string) ([]string, error) {
	data, err := os.ReadFile(manifest)
	if err != nil {
		return nil, err
	}
	var settings struct {
		Type    string `json:"type"`
		Word    string `json:"wake_word"`
		Model   string `json:"model"`
		Version int    `json:"version"`
		Micro   struct {
			Cutoff float64 `json:"probability_cutoff"`
			Window int     `json:"sliding_window_size"`
			Step   int     `json:"feature_step_size"`
		} `json:"micro"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	if settings.Type != "micro" || settings.Version != 2 || settings.Micro.Step != 10 ||
		!strings.EqualFold(settings.Word, word) || settings.Model != filepath.Base(model) ||
		settings.Micro.Cutoff <= 0 || settings.Micro.Cutoff >= 1 || settings.Micro.Window < 1 || settings.Micro.Window > 200 {
		return nil, errors.New("microWakeWord model and configuration do not match the requested word or supported format")
	}
	if _, err := os.Stat(model); err != nil {
		return nil, err
	}
	return []string{model, strings.ToLower(word), fmt.Sprint(settings.Micro.Cutoff), fmt.Sprint(settings.Micro.Window)}, nil
}

func (w *wakewordController) Start() error {
	w.startMu.Lock()
	defer w.startMu.Unlock()
	w.mu.Lock()
	if w.cancel != nil {
		wasPaused := w.paused
		w.triggered, w.paused = false, false
		if wasPaused {
			// Flush the neural model's audio history without feeding beep/question
			// audio. The next wake must come from fresh microphone samples.
			select {
			case w.frames <- make([]byte, 2*wakeSampleRate*2): // Two seconds of mono silence.
			default:
			}
			w.acceptWakeAfter = time.Now().Add(200 * time.Millisecond)
		}
		if w.decoderReady {
			w.capture.SetWakeSink(w.feed)
		}
		w.mu.Unlock()
		return nil
	}
	if _, err := os.Stat(w.binary); err != nil {
		w.mu.Unlock()
		return fmt.Errorf("local wake-word runtime %s: %w", w.binary, err)
	}
	args, err := microWakeArguments(w.model, w.manifest, w.word)
	if err != nil {
		w.mu.Unlock()
		return fmt.Errorf("local wake-word configuration: %w", err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	done := make(chan struct{})
	ready := make(chan struct{})
	frames := make(chan []byte, 32) // At most 640 ms; tolerate short playback CPU bursts.
	w.mu.Unlock()
	detector := exec.CommandContext(ctx, w.binary, args...)
	// Preserve startup errors in the client log. Normal recognizer output is
	// JSON on stdout, while stderr is useful only for diagnosis.
	detector.Stderr = os.Stderr
	input, err := detector.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	output, err := detector.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err = detector.Start(); err != nil {
		cancel()
		return fmt.Errorf("start wake-word detector: %w", err)
	}
	w.mu.Lock()
	w.cancel, w.done, w.input, w.triggered = cancel, done, input, false
	w.paused, w.decoderReady = false, false
	w.frames, w.droppedFrames = frames, 0
	w.mu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pcm := <-frames:
				w.mu.Lock()
				paused := w.paused
				w.mu.Unlock()
				if paused {
					continue
				}
				if _, err := input.Write(pcm); err != nil {
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "wake decoder input failed: %v\n", err)
						cancel()
					}
					return
				}
			}
		}
	}()

	go func() {
		defer close(done)
		defer func() {
			w.mu.Lock()
			if w.done == done {
				w.cancel, w.done, w.input = nil, nil, nil
				w.capture.SetWakeSink(nil)
			}
			w.mu.Unlock()
		}()
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			var result struct {
				Transcript string `json:"t"`
				Ready      bool   `json:"ready"`
			}
			if json.Unmarshal(scanner.Bytes(), &result) != nil {
				fmt.Fprintf(os.Stderr, "wake-word detector output: %s\n", scanner.Text())
				continue
			}
			if result.Ready {
				w.mu.Lock()
				if !w.decoderReady {
					w.decoderReady = true
					if !w.paused {
						w.capture.SetWakeSink(w.feed)
					}
					close(ready)
				}
				w.mu.Unlock()
				fmt.Printf("WAKE READY: native keyword detector loaded; listening for %q\n", w.word)
				continue
			}
			transcript := strings.TrimSpace(result.Transcript)
			if transcript != "" {
				fmt.Printf("WAKE WORD HEARD: %q\n", transcript)
			}
			if strings.EqualFold(transcript, w.word) {
				go w.activate()
			}
		}
		unexpectedExit := ctx.Err() == nil
		if unexpectedExit {
			cancel()
		}
		detectorErr := detector.Wait()
		if unexpectedExit {
			if detectorErr == nil {
				detectorErr = errors.New("unexpected end of detector output")
			}
			select {
			case w.errCh <- fmt.Errorf("wake-word detector stopped: %w", detectorErr):
			default:
			}
		}
	}()
	select {
	case <-ready:
	case <-done:
		return errors.New("wake detector exited before readiness")
	case <-time.After(10 * time.Second):
		cancel()
		return errors.New("wake detector did not initialize within 10 seconds")
	}
	fmt.Printf("MIC1 STANDBY: listening locally for %q (microWakeWord cutoff %s)\n", w.word, args[2])
	return nil
}

func (w *wakewordController) Stop() {
	w.startMu.Lock()
	defer w.startMu.Unlock()
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel == nil {
		return
	}
	w.capture.SetWakeSink(nil)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

func (w *wakewordController) feed(pcm []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.paused || w.frames == nil {
		return nil
	}
	select {
	case w.frames <- pcm:
		return nil
	default:
	}
	// Keep current audio instead of accumulating seconds of decoder backlog.
	select {
	case <-w.frames:
	default:
	}
	select {
	case w.frames <- pcm:
	default:
	}
	w.droppedFrames++
	if w.droppedFrames == 1 || w.droppedFrames%50 == 0 {
		fmt.Printf("WAKE AUDIO: decoder backlog; dropped old frames=%d\n", w.droppedFrames)
	}
	return nil
}

func (w *wakewordController) SetQuestionPending(pending bool) {
	w.mu.Lock()
	w.questionPending = pending
	w.mu.Unlock()
}

func (w *wakewordController) QuestionPending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.questionPending
}

func (w *wakewordController) Pause() {
	w.mu.Lock()
	w.paused = true
	w.capture.SetWakeSink(nil)
	if w.frames != nil {
		for {
			select {
			case <-w.frames:
			default:
				w.mu.Unlock()
				return
			}
		}
	}
	w.mu.Unlock()
}

func (w *wakewordController) activate() {
	w.mu.Lock()
	if w.triggered || w.paused || w.questionPending || time.Now().Before(w.acceptWakeAfter) {
		fmt.Printf("WAKE IGNORED: triggered=%t paused=%t question_pending=%t settling=%t\n",
			w.triggered, w.paused, w.questionPending, time.Now().Before(w.acceptWakeAfter))
		w.mu.Unlock()
		return
	}
	w.triggered = true
	w.questionPending = true
	w.mu.Unlock()
	w.Pause()
	w.capture.SetCloudEnabled(false)
	if w.interruptPlayback != nil {
		w.interruptPlayback()
	}
	if err := w.participant.PublishDataPacket(
		lksdk.UserData([]byte(`{"type":"interrupt_answer","source":"wake_word"}`)),
		lksdk.WithDataPublishTopic(controlTopic),
		lksdk.WithDataPublishReliable(true),
	); err != nil {
		fmt.Fprintf(os.Stderr, "wake interrupt control error: %v\n", err)
	}
	setWorkLED(true)
	if w.playBeep != nil {
		w.playBeep()
		// Wait for buffered beep audio before opening question capture.
		time.Sleep(wakeBeepDuration + wakeBeepSettle)
	}
	// The LED acknowledges the wake/beep only; it must not depend on a
	// later cloud response to turn off.
	setWorkLED(false)
	fmt.Println("WAKE ACK: LED off; beep wait complete")
	if err := w.capture.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start MIC1 question capture: %v\n", err)
		setWorkLED(false)
		w.SetQuestionPending(false)
		_ = w.Start()
		return
	}
	if err := w.capture.WaitReady(3 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "MIC1 question capture not ready: %v\n", err)
		w.SetQuestionPending(false)
		_ = w.Start()
		return
	}
	w.capture.SetCloudEnabled(true)
	activationPayload, _ := json.Marshal(map[string]string{
		"type": activationEvent, "source": "wake_word", "wake_word": w.word,
	})
	if err := w.participant.PublishDataPacket(
		lksdk.UserData(activationPayload),
		lksdk.WithDataPublishTopic(controlTopic),
		lksdk.WithDataPublishReliable(true),
	); err != nil {
		fmt.Fprintf(os.Stderr, "wake-word control event error: %v\n", err)
		w.capture.SetCloudEnabled(false)
		setWorkLED(false)
		w.SetQuestionPending(false)
		_ = w.Start()
		return
	}
	fmt.Println("WAKE WORD: beep complete; MIC1 is capturing one question for the cloud timer.")
}

func run() error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	logger.InitFromConfig(&logger.Config{Level: "info"}, "luckfox")
	lksdk.SetLogger(logger.GetLogger())

	disconnected := make(chan struct{})
	var disconnectOnce sync.Once
	var playbackMu sync.Mutex
	playbacks := make(map[string]*lkmedia.PCMRemoteTrack)
	speakers := make(map[string]*aplayWriter)
	var activeSpeaker *aplayWriter
	interruptPlayback := func() {
		playbackMu.Lock()
		for _, writer := range speakers {
			writer.Interrupt()
		}
		playbackMu.Unlock()
		fmt.Println("WAKE INTERRUPT: old AI playback stopped and buffered audio discarded")
	}
	playWakeTone := func() {
		playbackMu.Lock()
		writer := activeSpeaker
		playbackMu.Unlock()
		if err := playWakeBeep(writer); err == nil {
			return
		}
		// The remote AI speaker is sometimes not initialized yet when a local
		// wake word arrives. Open a short, independent ALSA stream so the user
		// always gets the audible wake acknowledgement.
		fallback, err := newAplayWriter(c.PlaybackDevice)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wake beep speaker error: %v\n", err)
			return
		}
		if err := playWakeBeep(fallback); err != nil {
			fmt.Fprintf(os.Stderr, "wake beep error: %v\n", err)
		}
		time.Sleep(wakeBeepDuration)
		_ = fallback.Close()
	}

	callbacks := &lksdk.RoomCallback{
		OnDisconnected: func() { disconnectOnce.Do(func() { close(disconnected) }) },
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed: func(track *webrtc.TrackRemote, _ *lksdk.RemoteTrackPublication, participant *lksdk.RemoteParticipant) {
				if track.Codec().MimeType != webrtc.MimeTypeOpus {
					return
				}
				fmt.Printf("Receiving voice from %s\n", participant.Identity())
				writer, writerErr := newAplayWriter(c.PlaybackDevice)
				if writerErr != nil {
					fmt.Fprintf(os.Stderr, "speaker error: %v\n", writerErr)
					return
				}
				pcmTrack, trackErr := lkmedia.NewPCMRemoteTrack(track, writer, lkmedia.WithTargetSampleRate(sampleRate), lkmedia.WithTargetChannels(outputChannels))
				if trackErr != nil {
					_ = writer.Close()
					fmt.Fprintf(os.Stderr, "remote audio error: %v\n", trackErr)
					return
				}
				playbackMu.Lock()
				playbacks[track.ID()] = pcmTrack
				speakers[track.ID()] = writer
				activeSpeaker = writer
				playbackMu.Unlock()
			},
			OnTrackUnsubscribed: func(track *webrtc.TrackRemote, _ *lksdk.RemoteTrackPublication, _ *lksdk.RemoteParticipant) {
				playbackMu.Lock()
				pcmTrack := playbacks[track.ID()]
				writer := speakers[track.ID()]
				delete(playbacks, track.ID())
				delete(speakers, track.ID())
				if activeSpeaker == writer {
					activeSpeaker = nil
				}
				playbackMu.Unlock()
				if pcmTrack != nil {
					pcmTrack.Close()
				}
			},
		},
	}

	var room *lksdk.Room
	var capture *captureController
	var wake *wakewordController
	var automation *automationBridge
	callbacks.ParticipantCallback.OnDataPacket = func(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
		packet, ok := data.(*lksdk.UserDataPacket)
		if !ok || packet.Topic != controlTopic {
			return
		}
		var control struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(packet.Payload, &control) != nil {
			return
		}
		if params.Sender == nil || params.Sender.Kind() != lksdk.ParticipantAgent {
			fmt.Fprintf(os.Stderr, "Ignored control packet from non-agent participant %s\n", params.SenderIdentity)
			return
		}
		if control.Type == micSleepEvent && capture != nil {
			capture.SetCloudEnabled(false)
			setWorkLED(false)
			if wake != nil {
				wake.SetQuestionPending(false)
				go func() {
					if wakeErr := wake.Start(); wakeErr != nil {
						fmt.Fprintf(os.Stderr, "restart local wake-word listener: %v\n", wakeErr)
					}
				}()
			}
		} else if control.Type == assistantSpeaking && capture != nil {
			if wake != nil && wake.QuestionPending() {
				return // An old answer must not overwrite the newly armed question.
			}
			capture.SetCloudEnabled(false)
			setWorkLED(false)
			playbackMu.Lock()
			for _, writer := range speakers {
				writer.Resume()
			}
			playbackMu.Unlock()
			if wake != nil {
				go func() {
					if err := wake.Start(); err != nil {
						fmt.Fprintf(os.Stderr, "start playback wake listener: %v\n", err)
					}
				}()
			}
			fmt.Printf("PLAYBACK: local %q listener remains active for interruption\n", c.WakeWord)
		} else if control.Type == assistantListening && capture != nil {
			if wake != nil && wake.QuestionPending() {
				return // Interrupting the old answer must not close the new question.
			}
			capture.SetCloudEnabled(false)
			setWorkLED(false)
			if wake != nil {
				go func() {
					if wakeErr := wake.Start(); wakeErr != nil {
						fmt.Fprintf(os.Stderr, "restart local wake-word listener: %v\n", wakeErr)
					}
				}()
			}
			fmt.Printf("STANDBY: AI reply ended; %q detector enabled\n", c.WakeWord)
		} else if control.Type == geckoRequestEvent && automation != nil {
			go automation.handle(packet.Payload, params.SenderIdentity)
		}
	}
	if c.Token != "" {
		room, err = lksdk.ConnectToRoomWithToken(c.URL, c.Token, callbacks, lksdk.WithDisableRegionDiscovery())
	} else {
		room, err = lksdk.ConnectToRoom(c.URL, lksdk.ConnectInfo{
			APIKey: c.APIKey, APISecret: c.APISecret, RoomName: c.Room, ParticipantIdentity: c.Identity,
		}, callbacks, lksdk.WithDisableRegionDiscovery())
	}
	if err != nil {
		return fmt.Errorf("connect to LiveKit: %w", err)
	}
	defer room.Disconnect()
	fmt.Printf("Connected to LiveKit room %s\n", c.Room)
	automation = newAutomationBridge(c, room.LocalParticipant)
	if c.GeckoLinkToken == "" && (c.GeckoLinkUsername == "" || c.GeckoLinkPassword == "") {
		fmt.Println("GeckoLink voice tools disabled: set token or username/password")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		gatewayURL, resolveErr := automation.resolver.URL(ctx, true)
		cancel()
		if resolveErr != nil {
			fmt.Printf("GeckoLink gateway %s is not currently reachable; commands will retry MAC resolution: %v\n", geckoLinkGatewayMAC, resolveErr)
		} else {
			fmt.Printf("GeckoLink voice bridge ready for gateway %s at %s\n", geckoLinkGatewayMAC, gatewayURL)
		}
	}
	if err = ensureAgent(c); err != nil {
		return err
	}

	micTrack, err := lkmedia.NewPCMLocalTrack(wakeSampleRate, 1, logger.GetLogger())
	if err != nil {
		return fmt.Errorf("create microphone track: %w", err)
	}
	defer micTrack.Close()
	_, err = room.LocalParticipant.PublishTrack(micTrack, &lksdk.TrackPublicationOptions{Name: "luckfox-microphone", Source: livekit.TrackSource_MICROPHONE})
	if err != nil {
		return fmt.Errorf("publish microphone track: %w", err)
	}
	fmt.Printf("Microphone track published; cloud capture starts only after local %q detection.\n", c.WakeWord)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	capture = newCaptureController(ctx, micTrack, c.CaptureDevice)
	defer capture.Stop()
	if err = capture.Start(); err != nil {
		return fmt.Errorf("start local MIC1 capture: %w", err)
	}
	if err = capture.WaitReady(3 * time.Second); err != nil {
		return err
	}
	wake = newWakewordController(ctx, room.LocalParticipant, capture, c, playWakeTone)
	wake.interruptPlayback = interruptPlayback
	defer wake.Stop()
	if err = wake.Start(); err != nil {
		return fmt.Errorf("start local %q detector: %w", c.WakeWord, err)
	}
	fmt.Printf("MIC1 standby is local only. Say %q, wait for the beep, then ask one question.\n", c.WakeWord)
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
		fmt.Println("Stopping")
	case <-disconnected:
		return errors.New("LiveKit disconnected")
	case captureErr := <-capture.errCh:
		if captureErr != nil {
			return captureErr
		}
	case wakeErr := <-wake.errCh:
		if wakeErr != nil {
			return wakeErr
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
