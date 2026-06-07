# Extending Corder

Corder extensions are executable plugins. A plugin registers one or more actions for selected recordings, and Corder runs the configured command as a subprocess when the user presses the action key.

Corder does not use Go native plugins.

## Manifest Locations

Corder loads plugin manifests from the user config directory:

```text
<config-dir>/corder/plugins/*.json
<config-dir>/corder/plugins/*/plugin.json
```

For example, on many Linux systems this is under:

```text
~/.config/corder/plugins/
```

Plugins are not loaded from the project directory by default.

## Manifest Format

```json
{
  "schema": 1,
  "id": "transcribe-openai",
  "name": "Transcribe with OpenAI",
  "version": "0.1.0",
  "actions": [
    {
      "id": "transcribe",
      "key": "T",
      "label": "transcribe",
      "command": "corder-transcribe-openai",
      "args": ["--file", "{{path}}", "--meta", "{{meta_path}}"],
      "formats": [".mp3", ".wav"],
      "job": true,
      "timeout_seconds": 1800
    }
  ]
}
```

Action fields:

- `id`: action identifier within the plugin.
- `key`: key shown in the footer and used to trigger the action.
- `label`: human-readable action label.
- `command`: executable name or path. Corder runs it directly, not through a shell.
- `args`: command arguments. Known template tokens are expanded.
- `formats`: optional list of lowercase file extensions, such as `.mp3` or `.wav`. Empty means all recording formats.
- `job`: when true, Corder runs the plugin asynchronously and shows progress in the recordings table.
- `timeout_seconds`: optional subprocess timeout. Omit or use `0` for no explicit timeout.

## Validation

Corder validates manifests at startup. Invalid manifests do not crash the app, and invalid plugin actions are disabled.

Built-in action keys always win. A plugin action is disabled if its key conflicts with a built-in action or an earlier loaded plugin action. Other validation errors include unsupported schema versions, missing plugin or action IDs, missing keys, missing commands, and invalid format extensions.

Extension issues are shown in the diagnostics screen.

## Argument Templates

Corder expands only these tokens in action arguments:

```text
{{path}}
{{meta_path}}
{{name}}
{{recording_dir}}
{{config_dir}}
```

Unknown tokens are left unchanged.

## Environment

Plugin commands receive these environment variables:

```text
CORDER_PLUGIN_ID
CORDER_ACTION_ID
CORDER_RECORDING_PATH
CORDER_META_PATH
CORDER_RECORDING_NAME
CORDER_RECORDING_DIR
CORDER_CONFIG_DIR
```

## Output Protocol

Plugins may write JSON lines to stdout. Each line should be one event:

```json
{"type":"status","message":"Transcribing"}
{"type":"progress","message":"Transcribing","percent":42}
{"type":"result","message":"Transcript saved","paths":["/recordings/a.txt"]}
{"type":"error","message":"OPENAI_API_KEY is not set"}
```

Supported event types:

- `status`: updates the current status text.
- `progress`: updates status text and progress percentage.
- `result`: marks the action done. The first path in `paths` is treated as the result path.
- `error`: marks the action failed.

If the process exits successfully without a `result` event, Corder marks the action complete. If the process exits with an error, Corder shows stderr when available.
