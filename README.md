# Walkie-Talkie Application

This is a simple walkie-talkie–style application written in Go that lets users on the same local network communicate using audio. When you press the button in the GUI, the application captures your microphone input and broadcasts it over UDP; other instances of the app on the same network will play the audio.

## Prerequisites

- **Go:** Version 1.20 or later.
- **PortAudio:**  
  - **Ubuntu/Debian:** `sudo apt-get install portaudio19-dev`
  - **macOS:** `brew install portaudio`
  - **Windows:** Download and install the PortAudio binaries from [PortAudio](http://www.portaudio.com/download.html).

## Setup and Running

1. **Clone the Repository**

    ```bash
        git clone https://github.com/yourusername/walkietalkie.git
        cd walkietalkie
    ```
2. **Download Dependencies**

    ```bash
        go mod tidy
    ```

3. **Run the Application**

    ```bash
        go run main.go
    ```