package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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
	sampleRate     = 48000
	inputChannels  = 2
	outputChannels = 2
	frameMillis    = 20
	buttonValue    = "/sys/class/gpio/gpio42/value"
	buttonTopic    = "luckfox-control"
	micSleepEvent  = "mic_sleep"
	buttonPoll     = 10 * time.Millisecond
	buttonDebounce = 40 * time.Millisecond
	buttonPullReg  = "0xff5381c4"
	buttonPullUp   = "0x300010"
)

type config struct {
	URL, APIKey, APISecret, Token string
	Room, Identity, AgentName     string
	CaptureDevice, PlaybackDevice string
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadConfig() (config, error) {
	c := config{
		URL:            os.Getenv("LIVEKIT_URL"),
		APIKey:         os.Getenv("LIVEKIT_API_KEY"),
		APISecret:      os.Getenv("LIVEKIT_API_SECRET"),
		Token:          os.Getenv("LIVEKIT_TOKEN"),
		Room:           env("LIVEKIT_ROOM", "luckfox-voice"),
		Identity:       env("LIVEKIT_IDENTITY", "luckfox-board"),
		AgentName:      env("LIVEKIT_AGENT_NAME", "my-agent"),
		CaptureDevice:  env("CAPTURE_DEVICE", "hw:0,0"),
		PlaybackDevice: env("PLAYBACK_DEVICE", "hw:0,0"),
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
}

func newAplayWriter(device string) (*aplayWriter, error) {
	cmd := exec.Command("aplay", "-q", "-D", device, "-t", "raw", "-f", "S16_LE", "-r", fmt.Sprint(sampleRate), "-c", fmt.Sprint(outputChannels))
	// The remote agent track is silent between replies. ALSA reports each
	// resulting empty playback interval as an underrun, which would otherwise
	// fill the device log even though later speech continues normally.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	return &aplayWriter{cmd: cmd, in: in}, nil
}

func (w *aplayWriter) WriteSample(sample media.PCM16Sample) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return io.ErrClosedPipe
	}
	buf := make([]byte, len(sample)*2)
	for i, value := range sample {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(value))
	}
	_, err := w.in.Write(buf)
	return err
}

func (w *aplayWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	err := w.in.Close()
	w.mu.Unlock()
	_ = w.cmd.Wait()
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

type captureController struct {
	ctx    context.Context
	track  *lkmedia.PCMLocalTrack
	device string
	errCh  chan error
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func newCaptureController(ctx context.Context, track *lkmedia.PCMLocalTrack, device string) *captureController {
	return &captureController{ctx: ctx, track: track, device: device, errCh: make(chan error, 1)}
}

func (c *captureController) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(c.ctx)
	cmd := exec.CommandContext(ctx, "arecord", "-q", "-D", c.device, "-t", "raw", "-f", "S16_LE", "-r", fmt.Sprint(sampleRate), "-c", fmt.Sprint(inputChannels))
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
	c.cancel = cancel
	c.done = done
	fmt.Printf("MIC1 ON: capturing %s\n", c.device)

	go func() {
		defer close(done)
		frameSamples := sampleRate * frameMillis / 1000
		raw := make([]byte, frameSamples*inputChannels*2)
		mono := make(media.PCM16Sample, frameSamples)
		var captureErr error
		for {
			_, readErr := io.ReadFull(out, raw)
			if readErr != nil {
				if ctx.Err() == nil {
					captureErr = fmt.Errorf("read microphone: %w", readErr)
				}
				break
			}
			for i := 0; i < frameSamples; i++ {
				left := int16(binary.LittleEndian.Uint16(raw[i*4:]))
				right := int16(binary.LittleEndian.Uint16(raw[i*4+2:]))
				mono[i] = int16((int32(left) + int32(right)) / 2)
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
	fmt.Println("MIC1 OFF: standby")
}

func readButtonPressed() (bool, error) {
	value, err := os.ReadFile(buttonValue)
	if err != nil {
		return false, err
	}
	// The installed normally-closed switch grounds GPIO42 while released and
	// opens it when pressed, so its observed pressed level is HIGH.
	return strings.TrimSpace(string(value)) == "1", nil
}

func startButton(ctx context.Context, participant *lksdk.LocalParticipant, capture *captureController) (<-chan error, error) {
	if output, err := exec.Command("iomux", "1", "10", "0").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("configure GPIO42 pinmux: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("io", "-4", buttonPullReg, buttonPullUp).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("enable GPIO42 pull-up: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(buttonValue); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect GPIO42: %w", err)
		}
		if err = os.WriteFile("/sys/class/gpio/export", []byte("42"), 0o644); err != nil {
			return nil, fmt.Errorf("export GPIO42: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := os.WriteFile("/sys/class/gpio/gpio42/direction", []byte("in"), 0o644); err != nil {
		return nil, fmt.Errorf("configure GPIO42 input: %w", err)
	}

	pressed, err := readButtonPressed()
	if err != nil {
		return nil, fmt.Errorf("read GPIO42: %w", err)
	}
	level := "LOW"
	if pressed {
		level = "HIGH"
	}
	fmt.Printf("Push button ready on GPIO42; released=LOW, pressed=HIGH. Current level=%s\n", level)

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		ticker := time.NewTicker(buttonPoll)
		defer ticker.Stop()
		stable, candidate := pressed, pressed
		changedAt := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, readErr := readButtonPressed()
				if readErr != nil {
					errCh <- fmt.Errorf("read GPIO42: %w", readErr)
					return
				}
				if current != candidate {
					candidate = current
					changedAt = time.Now()
				}
				if candidate == stable || time.Since(changedAt) < buttonDebounce {
					continue
				}
				stable = candidate
				if !stable {
					continue
				}
				if captureErr := capture.Start(); captureErr != nil {
					fmt.Fprintf(os.Stderr, "start MIC1 error: %v\n", captureErr)
					continue
				}
				if publishErr := participant.PublishDataPacket(
					lksdk.UserData([]byte(`{"type":"arm_question"}`)),
					lksdk.WithDataPublishTopic(buttonTopic),
					lksdk.WithDataPublishReliable(true),
				); publishErr != nil {
					fmt.Fprintf(os.Stderr, "push button event error: %v\n", publishErr)
					capture.Stop()
					continue
				}
				fmt.Println("Push button pressed; event sent reliably; ask one question.")
			}
		}
	}()
	return errCh, nil
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
				playbackMu.Unlock()
			},
			OnTrackUnsubscribed: func(track *webrtc.TrackRemote, _ *lksdk.RemoteTrackPublication, _ *lksdk.RemoteParticipant) {
				playbackMu.Lock()
				pcmTrack := playbacks[track.ID()]
				delete(playbacks, track.ID())
				playbackMu.Unlock()
				if pcmTrack != nil {
					pcmTrack.Close()
				}
			},
		},
	}

	var room *lksdk.Room
	var capture *captureController
	callbacks.ParticipantCallback.OnDataPacket = func(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
		packet, ok := data.(*lksdk.UserDataPacket)
		if !ok || packet.Topic != buttonTopic || capture == nil {
			return
		}
		var control struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(packet.Payload, &control) == nil && control.Type == micSleepEvent {
			capture.Stop()
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
	if err = ensureAgent(c); err != nil {
		return err
	}

	micTrack, err := lkmedia.NewPCMLocalTrack(sampleRate, 1, logger.GetLogger())
	if err != nil {
		return fmt.Errorf("create microphone track: %w", err)
	}
	defer micTrack.Close()
	_, err = room.LocalParticipant.PublishTrack(micTrack, &lksdk.TrackPublicationOptions{Name: "luckfox-microphone", Source: livekit.TrackSource_MICROPHONE})
	if err != nil {
		return fmt.Errorf("publish microphone track: %w", err)
	}
	fmt.Println("Microphone track published; MIC1 capture starts only after a button press.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	capture = newCaptureController(ctx, micTrack, c.CaptureDevice)
	defer capture.Stop()
	buttonErrors, err := startButton(ctx, room.LocalParticipant, capture)
	if err != nil {
		return fmt.Errorf("start push button: %w", err)
	}
	fmt.Println("MIC1 OFF: standby; press the button to start capture.")
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
	case buttonErr := <-buttonErrors:
		if buttonErr != nil {
			return buttonErr
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
