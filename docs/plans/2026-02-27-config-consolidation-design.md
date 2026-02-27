# Config Consolidation Design

## Summary

Consolidate all configuration into a TOML config file (`~/.argus/config.toml`), replace manual TOML parsing with Viper, support environment variable overrides with `ARGUS_` prefix, and remove `--port`/`--db` CLI flags in favor of config-file-only settings.

## Config File Structure

Location: `~/.argus/config.toml` (default), overridable via `--config`.

```toml
[server]
port = 3000
bind_address = "127.0.0.1"

[agent]
port = 3011
bind_address = "127.0.0.1"

[database]
path = "~/.argus/agent.db"

[git]
branch_prefix = ""
```

## Go Config Struct

```go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Agent    AgentConfig    `mapstructure:"agent"`
    Database DatabaseConfig `mapstructure:"database"`
    Git      GitConfig      `mapstructure:"git"`
}

type ServerConfig struct {
    Port        int    `mapstructure:"port"`
    BindAddress string `mapstructure:"bind_address"`
}

type AgentConfig struct {
    Port        int    `mapstructure:"port"`
    BindAddress string `mapstructure:"bind_address"`
}

type DatabaseConfig struct {
    Path string `mapstructure:"path"`
}

type GitConfig struct {
    BranchPrefix string `mapstructure:"branch_prefix"`
}
```

## Command-to-Config Mapping

| Command | Port source | Bind address source |
|---------|------------|-------------------|
| `argus` (combined) | `server.port` (3000) | `server.bind_address` (127.0.0.1) |
| `argus server` | `server.port` (3000) | `server.bind_address` (127.0.0.1) |
| `argus agent` | `agent.port` (3011) | `agent.bind_address` (127.0.0.1) |

Combined mode runs both SPA frontend and agent API on a single HTTP server using the `[server]` config block. The `[agent]` section only applies when running standalone via `argus agent`.

## CLI Changes

**Removed flags:** `--port` and `--db` from root, server, and agent commands.

**New persistent flag on root:**
- `--config STRING` — Path to config file. Overrides the default `~/.argus/config.toml` entirely.

**Kept flags:**
- `--web` on `server` subcommand (dev-only, CLI-only)
- `--provider`, `--src`, `--yolo` on `session new` (unchanged)

## Viper Integration

Initialization via `cobra.OnInitialize(initConfig)`:

1. Set defaults for all keys via `viper.SetDefault()`
2. Set env prefix `ARGUS`, enable `AutomaticEnv()`
3. Set env key replacer: `.` -> `_` (so `server.port` maps to `ARGUS_SERVER_PORT`)
4. If `--config` is set, use `viper.SetConfigFile(path)`
5. Otherwise, add `~/.argus/` as search path, name `config`, type `toml`
6. Call `viper.ReadInConfig()` — missing file is not an error
7. Unmarshal into the global `Config` struct

## Environment Variable Mapping

| Config key | Env var |
|-----------|---------|
| `server.port` | `ARGUS_SERVER_PORT` |
| `server.bind_address` | `ARGUS_SERVER_BIND_ADDRESS` |
| `agent.port` | `ARGUS_AGENT_PORT` |
| `agent.bind_address` | `ARGUS_AGENT_BIND_ADDRESS` |
| `database.path` | `ARGUS_DATABASE_PATH` |
| `git.branch_prefix` | `ARGUS_GIT_BRANCH_PREFIX` |

## Priority Order (highest to lowest)

1. Environment variables
2. Config file
3. Defaults

## Bind Address Behavior

Both `server.bind_address` and `agent.bind_address` default to `127.0.0.1` (loopback). This is enforced via `viper.SetDefault()`, so if the config file is missing, empty, or does not specify a bind address, the server binds to loopback only. This addresses the existing security TODO about not binding to all interfaces.

The listen address is constructed as `fmt.Sprintf("%s:%d", bindAddress, port)`.

## Discovery File

`writeDiscovery()` writes the actual listen address to `~/.argus/agent.json`. The existing normalization of wildcard addresses (`0.0.0.0`, `[::]`) to `127.0.0.1` remains as a safety fallback. The loopback validation in `readDiscovery()` stays unchanged.

## Dependency Changes

- **Add:** `github.com/spf13/viper` (with `github.com/pelletier/go-toml/v2` for TOML support)
- **Remove:** `github.com/BurntSushi/toml`

## Error Handling

- Missing config file: silently use defaults (lenient, matches current behavior)
- Malformed TOML: fail startup with a clear error message
- Invalid values: validate after unmarshal, fail with descriptive errors

## Validation Rules

- `port` must be 1-65535
- `bind_address` must be a valid IP (via `net.ParseIP`)
- `database.path` must be non-empty after `~` expansion

## Testing

- Unit tests for config loading: defaults, file overrides, env var overrides, priority ordering
- Unit tests for validation: invalid port, invalid bind address, empty db path
- Integration test: full startup with a custom config file via `--config`
