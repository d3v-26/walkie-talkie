package main

import (
	"context"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 512
	udpPort         = 3000
	// The header is 2 int16 values (4 bytes):
	headerSizeBytes = 4
	// The UDP buffer must now accommodate the header plus audio data.
	bufferSize = framesPerBuffer*2 + headerSizeBytes
)

var (
	broadcastAddr = net.UDPAddr{IP: net.IPv4bcast, Port: udpPort}
	transmitting  bool
	transmitMutex sync.RWMutex

	selfID       int16      // Unique ID for this instance.
	headerMarker int16 = 0x7BCD // Marker to identify our packet header.
)

// RingBuffer structure optimized
type RingBuffer struct {
	buf       []int16
	size      int
	readPos   int
	writePos  int
	available int
	mu        sync.Mutex
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]int16, size),
		size: size,
	}
}

func (r *RingBuffer) Write(samples []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range samples {
		if r.available < r.size {
			r.buf[r.writePos] = s
			r.writePos = (r.writePos + 1) % r.size
			r.available++
		} else {
			log.Println("RingBuffer overflow, sample dropped")
			break
		}
	}
}

func (r *RingBuffer) Read(out []int16) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for count < len(out) && r.available > 0 {
		out[count] = r.buf[r.readPos]
		r.readPos = (r.readPos + 1) % r.size
		r.available--
		count++
	}
	return count
}

func main() {
	// Generate a unique ID for this instance.
	rand.Seed(time.Now().UnixNano())
	selfID = int16(rand.Intn(65536))

	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("PortAudio initialization failed: %v", err)
	}
	defer portaudio.Terminate()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ringBuffer := NewRingBuffer(framesPerBuffer * 20)

	go receiveAudio(ctx, ringBuffer)
	go playbackAudio(ctx, ringBuffer)

	startGUI(ctx, cancel)
}

// GUI with Fyne
func startGUI(ctx context.Context, cancel context.CancelFunc) {
	a := app.New()
	w := a.NewWindow("Walkie Talkie")

	var button *widget.Button
	button = widget.NewButton("Push to Talk", func() {
		transmitMutex.Lock()
		transmitting = !transmitting
		active := transmitting
		transmitMutex.Unlock()

		if active {
			button.SetText("Stop Talking")
			go transmitAudio(ctx)
		} else {
			button.SetText("Push to Talk")
		}
	})

	w.SetContent(container.NewCenter(button))
	w.Resize(fyne.NewSize(200, 100))
	w.SetOnClosed(func() { cancel() })
	w.ShowAndRun()
}

// transmitAudio handles microphone capture and UDP broadcast.
// It prepends a header [marker, selfID] to each packet.
func transmitAudio(ctx context.Context) {
	conn, err := net.DialUDP("udp4", nil, &broadcastAddr)
	if err != nil {
		log.Printf("UDP dial error: %v", err)
		return
	}
	defer conn.Close()

	in := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(in), in)
	if err != nil {
		log.Printf("Input stream error: %v", err)
		return
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Printf("Input stream start error: %v", err)
		return
	}
	defer stream.Stop()

	log.Println("Transmitting started.")
	for {
		select {
		case <-ctx.Done():
			log.Println("Transmit context cancelled")
			return
		default:
			transmitMutex.RLock()
			active := transmitting
			transmitMutex.RUnlock()

			if !active {
				log.Println("Stopped transmitting by user.")
				return
			}

			if err := stream.Read(); err != nil {
				log.Printf("Audio read error: %v", err)
				continue
			}

			// Create packet: header ([marker, selfID]) + audio samples.
			header := []int16{headerMarker, selfID}
			packet := append(int16ToBytes(header), int16ToBytes(in)...)

			if _, err := conn.Write(packet); err != nil {
				log.Printf("UDP send error: %v", err)
			}
		}
	}
}

// receiveAudio listens for UDP packets and writes audio data to the RingBuffer.
// It skips packets that originate from this instance.
func receiveAudio(ctx context.Context, rb *RingBuffer) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: udpPort})
	if err != nil {
		log.Fatalf("UDP listen error: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, bufferSize)
	log.Println("Receiving audio...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Receive context cancelled")
			return
		default:
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("UDP read error: %v", err)
				continue
			}

			if n < headerSizeBytes {
				log.Println("Packet too short, ignoring.")
				continue
			}

			// Extract header.
			headerSamples := bytesToInt16(buf[:headerSizeBytes])
			if headerSamples[0] != headerMarker {
				log.Println("Invalid header marker, ignoring packet.")
				continue
			}
			// Ignore packets sent by this instance.
			if headerSamples[1] == selfID {
				// Optionally log: log.Printf("Ignored own packet from %v", addr)
				continue
			}

			// Write the audio data (excluding the header) to the ring buffer.
			audioData := buf[headerSizeBytes:n]
			rb.Write(bytesToInt16(audioData))
		}
	}
}

// playbackAudio outputs audio from RingBuffer.
func playbackAudio(ctx context.Context, rb *RingBuffer) {
	out := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(0, 1, sampleRate, len(out), &out)
	if err != nil {
		log.Fatalf("Output stream error: %v", err)
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Fatalf("Output stream start error: %v", err)
	}
	defer stream.Stop()

	log.Println("Playback started.")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Playback context cancelled")
			return
		case <-ticker.C:
			n := rb.Read(out)
			if n < len(out) {
				// Fill the remainder with silence.
				for i := n; i < len(out); i++ {
					out[i] = 0
				}
			}
			if err := stream.Write(); err != nil {
				log.Printf("Playback error: %v", err)
			}
		}
	}
}

// Helpers: convert between int16 slices and byte slices.
func int16ToBytes(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		buf[2*i] = byte(s)
		buf[2*i+1] = byte(s >> 8)
	}
	return buf
}

func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(data[2*i]) | int16(data[2*i+1])<<8
	}
	return samples
}
