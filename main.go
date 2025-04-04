package main

import (
	"bytes"
	"log"
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
	sampleRate      = 16000         // Samples per second (mono)
	framesPerBuffer = 512           // Number of samples per buffer (adjusted to fit MTU)
	udpPort         = 3000          // UDP port for both sending and receiving
)

// Broadcast address for the local network (IPv4 broadcast)
var broadcastAddr = net.UDPAddr{IP: net.IPv4bcast, Port: udpPort}

// Global flag to control transmission.
var (
	transmitting  = false
	transmitMutex sync.Mutex
)

// RingBuffer holds int16 samples.
type RingBuffer struct {
	buf       []int16
	size      int
	readPos   int
	writePos  int
	available int
	mu        sync.Mutex
}

// NewRingBuffer creates a new ring buffer of the given size.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]int16, size),
		size: size,
	}
}

// Write writes samples into the ring buffer.
func (r *RingBuffer) Write(samples []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range samples {
		if r.available < r.size {
			r.buf[r.writePos] = s
			r.writePos = (r.writePos + 1) % r.size
			r.available++
		} else {
			// Buffer full; you might log or drop extra samples.
		}
	}
}

// Read reads up to len(buffer) samples from the ring buffer into buffer.
// It returns the number of samples read.
func (r *RingBuffer) Read(buffer []int16) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for i := range buffer {
		if r.available > 0 {
			buffer[i] = r.buf[r.readPos]
			r.readPos = (r.readPos + 1) % r.size
			r.available--
			count++
		} else {
			break
		}
	}
	return count
}

func main() {
	// Initialize PortAudio.
	if err := portaudio.Initialize(); err != nil {
		log.Fatal("Error initializing PortAudio:", err)
	}
	defer portaudio.Terminate()

	// Create a ring buffer that can hold about 10 audio buffers.
	ringBuffer := NewRingBuffer(framesPerBuffer * 10)

	// Start UDP receiver in a goroutine, passing the ring buffer.
	go receiveAudio(ringBuffer)

	// Start a playback goroutine.
	go playbackAudio(ringBuffer)

	// Create the GUI using Fyne.
	a := app.New()
	w := a.NewWindow("Walkie Talkie")

	// Pre-declare the button variable.
	var button *widget.Button
	button = widget.NewButton("Push to Talk", func() {
		transmitMutex.Lock()
		transmitting = !transmitting
		active := transmitting
		transmitMutex.Unlock()

		if active {
			button.SetText("Stop Talking")
			go transmitAudio() // Start transmitting in a goroutine.
		} else {
			button.SetText("Push to Talk")
		}
	})

	w.SetContent(container.NewCenter(button))
	w.Resize(fyne.NewSize(200, 100))
	w.ShowAndRun()
}

// transmitAudio captures audio from the microphone and sends it as UDP packets.
func transmitAudio() {
	conn, err := net.DialUDP("udp4", nil, &broadcastAddr)
	if err != nil {
		log.Println("Error dialing UDP:", err)
		return
	}
	defer conn.Close()

	in := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(in), in)
	if err != nil {
		log.Println("Error opening input stream:", err)
		return
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Println("Error starting input stream:", err)
		return
	}
	defer stream.Stop()

	log.Println("Started transmitting audio...")
	for {
		transmitMutex.Lock()
		active := transmitting
		transmitMutex.Unlock()
		if !active {
			break
		}
		if err := stream.Read(); err != nil {
			log.Println("Error reading from input stream:", err)
			break
		}
		buf := int16SliceToBytes(in)
		if _, err := conn.Write(buf); err != nil {
			log.Println("Error sending UDP packet:", err)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	log.Println("Stopped transmitting audio.")
}

// receiveAudio listens on the UDP port for incoming audio, decodes it, and writes to the ring buffer.
func receiveAudio(rb *RingBuffer) {
	addr := net.UDPAddr{
		Port: udpPort,
		IP:   net.IPv4zero,
	}
	conn, err := net.ListenUDP("udp4", &addr)
	if err != nil {
		log.Println("Error starting UDP listener:", err)
		return
	}
	defer conn.Close()

	// Use a buffer size matching the packet size (framesPerBuffer * 2 bytes per sample).
	buf := make([]byte, framesPerBuffer*2)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Error receiving UDP packet:", err)
			continue
		}
		samples := bytesToInt16Slice(buf[:n])
		rb.Write(samples)
	}
}

// playbackAudio continuously reads audio samples from the ring buffer and writes them to the output stream.
func playbackAudio(rb *RingBuffer) {
	out := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(0, 1, sampleRate, len(out), &out)
	if err != nil {
		log.Println("Error opening output stream:", err)
		return
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Println("Error starting output stream:", err)
		return
	}
	defer stream.Stop()

	log.Println("Playback started.")
	for {
		// Read samples from the ring buffer.
		n := rb.Read(out)
		// If fewer than expected, fill remainder with silence.
		if n < len(out) {
			for i := n; i < len(out); i++ {
				out[i] = 0
			}
		}
		if err := stream.Write(); err != nil {
			log.Println("Error writing to output stream:", err)
		}
		// You may adjust the sleep duration if needed.
		time.Sleep(10 * time.Millisecond)
	}
}

// int16SliceToBytes converts a slice of int16 samples to a byte slice (little-endian).
func int16SliceToBytes(samples []int16) []byte {
	buf := new(bytes.Buffer)
	for _, s := range samples {
		buf.WriteByte(byte(s))
		buf.WriteByte(byte(s >> 8))
	}
	return buf.Bytes()
}

// bytesToInt16Slice converts a byte slice to a slice of int16 samples (little-endian).
func bytesToInt16Slice(b []byte) []int16 {
	length := len(b) / 2
	samples := make([]int16, length)
	for i := 0; i < length; i++ {
		samples[i] = int16(b[2*i]) | int16(b[2*i+1])<<8
	}
	return samples
}
