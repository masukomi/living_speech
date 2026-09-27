# LivingSpeech

A macOS menu bar text-to-speech app for the [OpenVox](https://openvox.app) local API, built with Go and Wails v3.

- Click the menu bar waveform to show or hide the panel (Esc also hides it; right-click for Quit).
- Return speaks the text and clears the box; Shift+Return adds a new line.
- Phrases from the last 24 hours appear under **Recent**. Click one to put it back in the box.
- The gear opens settings for model, language, and voice. The lists are fetched fresh each time.

Settings and recent phrases are stored in `~/Library/Application Support/LivingSpeech/`.

## Development

```bash
wails3 dev          # live-reload dev build
wails3 build        # production binary in bin/
go test ./...       # unit tests
OPENVOX_LIVE=1 go test -run Live -v ./internal/openvox   # against a running OpenVox
```
