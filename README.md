# Mattermost Upstage Document Parser Plugin

This plugin connects Mattermost messages and uploaded files to the Upstage Document Parser API. Admins can configure multiple Mattermost bot accounts, each with its own parser endpoint, auth settings, and parsing options.

## What It Does

- Receives a Mattermost message plus attached files
- Sends the files to the Upstage Document Parser API
- Returns the parsing result back into the channel or thread
- Supports multiple parser bots with different input parameters
- Lets admins override URL, auth mode, auth token, and bot access rules per bot

## Main Capabilities

- Multiple Mattermost bot accounts managed by the plugin
- Per-bot parser options such as `model`, `mode`, `ocr`, `output_formats`, `coordinates`, `chart_recognition`, `merge_multipage_tables`, and `base64_encoding`
- Global service configuration plus per-bot overrides
- Mattermost RHS workflow for choosing a bot and running document parsing
- Access control by user, team, and channel

## Project Layout

- `server/`: Go plugin server for Mattermost hooks, bot execution, parser API calls, and admin endpoints
- `webapp/`: React admin console UI and Mattermost RHS integration
- `build/`: Manifest and packaging helpers

## Configuration Model

The plugin stores its current configuration in the `Config` JSON field. A typical structure looks like this:

```json
{
  "service": {
    "base_url": "https://api.upstage.ai/v1/document-digitization",
    "auth_mode": "bearer",
    "auth_token": "YOUR_API_KEY",
    "allow_hosts": "api.upstage.ai"
  },
  "runtime": {
    "default_timeout_seconds": 30,
    "max_input_length": 4000,
    "max_output_length": 8000,
    "enable_debug_logs": false,
    "enable_usage_logs": true
  },
  "bots": [
    {
      "username": "doc-parser-bot",
      "display_name": "Document Parser",
      "model": "document-parse",
      "mode": "default",
      "ocr": "auto",
      "output_formats": [
        "markdown"
      ]
    }
  ]
}
```

Legacy Mattermost plugin settings are still read for backward compatibility, but new development should use the structured `Config` payload.

## Development

Server tests:

```bash
go test ./server/...
```

Webapp type check:

```bash
cd webapp
npm run check-types
```

Webapp build:

```bash
cd webapp
npm run build
```

Webapp tests:

```bash
cd webapp
npm run test -- --runInBand
```
