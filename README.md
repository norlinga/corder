# Corder

Corder is a minimal terminal audio recorder written in Go. It is built around a single keyboard-driven workflow: launch the app, record audio, stop when finished, and manage the resulting recordings from the same TUI.

The app uses Bubble Tea for the terminal interface, PortAudio for audio capture, and ffmpeg for MP3 conversion.

Build and development notes are in [BUILD.md](BUILD.md).

Corder is extendable with executable plugins that add actions for selected recordings, such as transcription or upload workflows. See [EXTENDING.md](EXTENDING.md) for the plugin manifest format and execution protocol.

## License

Corder is licensed under the MIT License. See [LICENSE](LICENSE).

Third-party dependencies are distributed under their own licenses.
