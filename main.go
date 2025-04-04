package main

import (
	"bytes"
	"log"
	"net"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/gordonklaus/portaudio"
)

const (
	sampleRate      = 16000 // Samples per second (mono)
	framesPerBuffer = 512   // Number of samples per buffer
	udpPort         = 3000  // UDP port for both sending and receiving
)

var (
	// Broadcast address for the local network (IPv4 broadcast)
	broadcastAddr = net.UDPAddr{IP: net.IPv4bcast, Port: udpPort}
)

var (
	// Global flag (protected by a mutex) to control transmission.
	transmitting  = false
	transmitMutex sync.Mutex
)

func main() {
	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		log.Fatal("Error initializing PortAudio:", err)
	}
	defer portaudio.Terminate()

	// Start a goroutine to listen for incoming audio on UDP
	go receiveAudio()

	// Create the GUI using Fyne
	a := app.New()
	w := a.NewWindow("Walkie Talkie")

	// Pre-declare the button variable
	var button *widget.Button
	button = widget.NewButton("Push to Talk", func() {
		transmitMutex.Lock()
		transmitting = !transmitting
		active := transmitting
		transmitMutex.Unlock()

		if active {
			button.SetText("Stop Talking")
			// Start transmitting in a separate goroutine
			go transmitAudio()
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
	// Set up a UDP connection for broadcast
	conn, err := net.DialUDP("udp4", nil, &broadcastAddr)
	if err != nil {
		log.Println("Error dialing UDP:", err)
		return
	}
	defer conn.Close()

	// Prepare the input buffer and open an audio stream for input
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
	// Continuously read from the microphone and send over UDP
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
		// Convert the int16 samples to bytes
		buf := int16SliceToBytes(in)
		if _, err := conn.Write(buf); err != nil {
			log.Println("Error sending UDP packet:", err)
			break
		}
		// Sleep briefly to yield CPU time
		time.Sleep(10 * time.Millisecond)
	}
	log.Println("Stopped transmitting audio.")
}

// receiveAudio listens on the UDP port for incoming audio and plays it.
func receiveAudio() {
	// Listen on all interfaces on udpPort
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

	// Prepare the output buffer and open an audio stream for playback
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

	log.Println("Listening for incoming audio...")
	buf := make([]byte, framesPerBuffer*2) // Expecting framesPerBuffer samples (2 bytes each)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Error receiving UDP packet:", err)
			continue
		}
		// Convert received bytes into int16 samples.
		samples := bytesToInt16Slice(buf[:n])
		// If fewer samples than expected, fill remaining with zeros.
		if len(samples) < len(out) {
			copy(out, samples)
			for i := len(samples); i < len(out); i++ {
				out[i] = 0 // fill missing samples with silence
			}
		} else {
			copy(out, samples)
		}
		if err := stream.Write(); err != nil {
			log.Println("Error writing to output stream:", err)
		}
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
