# Fix Plan

## Goal

Stabilize the app as a safe, responsive, root-capable Slackware TUI before adding more features.

## Priority Matrix

| Priority | Area | Impact | Effort |
|---|---|---:|---:|
| 1 | Fix definite crashes and terminal recovery | Very high | Low |
| 2 | Remove blocking work from the UI/render path | Very high | Medium |
| 3 | Centralize privileged command and file operations | Very high | Medium |
| 4 | Add tests for config rewrites, parsing, and TUI rendering | High | Medium |
| 5 | Harden Slackware-specific workflows | High | Medium |
| 6 | Add CI and reproducible release hygiene | High | Low |
| 7 | Reduce product/documentation drift and trim dead code | Medium | Low |
| 8 | Re-scope or defer incomplete tabs | Medium | Medium |

## Phase 0: Stop the Bleeding

Target: 1-2 days

### 1. Fix the config editor panic

- File: `src/components/config_editor.rs`
- Action: render status in `chunks[2]`, not `chunks[3]`
- Success criteria: opening F6 never panics

### 2. Add terminal panic recovery

- File: `src/main.rs`
- Action: install a panic hook that always disables raw mode, leaves alternate screen, and restores cursor before printing panic info
- Success criteria: forced panic leaves terminal usable

### 3. Remove command execution from render functions

- File: `src/components/network.rs`
- Action: cache gateway/default-route data in component state during refresh/activate, never in `render`
- Success criteria: render path is pure UI

### 4. Add a no-regression smoke pass for every tab

- Files: `src/components/*`, `src/app.rs`
- Action: instantiate each component and render it once with `ratatui::backend::TestBackend`
- Success criteria: every tab renders without panic

## Phase 1: Make the App Actually Responsive

Target: 3-5 days

### 1. Move long-running actions off the event loop

- Files: `src/main.rs`, `src/app.rs`, `src/slackware/commands.rs`
- Action: use `tokio::spawn` or `spawn_blocking` for blocking/system work; return messages via channels
- Success criteria: UI remains interactive during `slackpkg`, `sbopkg`, package removal, network restart, `lilo`, and similar actions

### 2. Wire the existing progress channel properly or remove it

- Files: `src/app.rs`, `src/slackware/commands.rs`
- Action:
  - create executor with `with_progress`
  - stream stdout/stderr line-by-line where possible
  - route updates by task ID instead of current tab
- Success criteria: progress output appears while commands run and is not lost when the user switches tabs

### 3. Standardize async task lifecycle

- Files: `src/components/mod.rs`, `src/app.rs`
- Action: replace ad hoc per-tab execution with a shared task model:
  - task start
  - task progress
  - task completion
  - task error
- Success criteria: no component directly blocks in `handle_input`

## Phase 2: Safety for Root-Level Operations

Target: 4-6 days

### 1. Centralize all command execution

- Files:
  - `src/slackware/commands.rs`
  - `src/components/services.rs`
  - `src/components/package_browser.rs`
  - `src/components/kernel.rs`
  - `src/components/network.rs`
- Action: remove direct `std::process::Command` calls from components and route all privileged ops through one executor API
- Success criteria: one place for logging, error formatting, timeouts, and future policy checks

### 2. Use atomic writes for system config files

- Files:
  - `src/components/config_editor.rs`
  - `src/slackware/config.rs`
  - `src/components/kernel.rs`
  - `src/components/settings.rs`
- Action: write to a temp file in the same directory, fsync, rename, optionally create a `.bak`
- Success criteria: partial writes and corrupted config files are avoided

### 3. Harden backups containing secrets

- File: `src/components/backup.rs`
- Action:
  - set backup directory permissions to `0700`
  - set backup file permissions to `0600`
  - make secret-file backup opt-in
- Success criteria: `/etc/shadow` backups are not world-readable

### 4. Reduce unnecessary root requirement

- File: `src/main.rs`
- Action: allow read-only startup without root; gate mutating actions individually
- Success criteria: logs, sysinfo, and package browsing work unprivileged

## Phase 3: Slackware Workflow Correctness

Target: 3-4 days

### 1. Harden mirror selection logic

- Files: `src/slackware/config.rs`, `src/app.rs`
- Action:
  - verify selected mirror exists before rewrite
  - ensure exactly one active mirror after rewrite
  - abort on zero/multiple active mirrors
  - back up the original mirrors file before mutation
- Success criteria: impossible to silently leave `slackpkg` misconfigured

### 2. Improve updater flow for real Slackware maintenance

- File: `src/components/updater.rs`
- Action:
  - add `show-changelog`
  - surface `.new` config file reminders
  - show blacklist status
  - distinguish stable release vs `-current`
  - tailor bootloader guidance for GRUB and LILO
- Success criteria: updater reflects Slackware's actual operational workflow

### 3. Surface package-search failures explicitly

- Files: `src/slackware/packages.rs`, `src/app.rs`
- Action: return `Result<Vec<PackageInfo>, Error>` instead of an empty vec on failure
- Success criteria: UI can say "sbotools not installed" vs "0 matches"

## Phase 4: Testing and Quality Gates

Target: 3-5 days

### 1. Add unit tests for parsers and config mutation

- Files:
  - `src/slackware/version.rs`
  - `src/slackware/config.rs`
  - `src/slackware/packages.rs`
  - `src/components/updater.rs`
- Action:
  - mirror parsing
  - mirror rewrite behavior
  - package-name parsing
  - `sbofind` output parsing
  - updater kernel-detection logic
- Success criteria: mutation-heavy logic is covered by deterministic tests

### 2. Add render tests for each component

- Action: use `ratatui::backend::TestBackend` to render every tab in representative states
- Success criteria: layout regressions are caught before release

### 3. Add integration tests around task/message flow

- File: `src/app.rs`
- Action: test message dispatch, task completion, and tab-switching while background work is active
- Success criteria: no output routing bugs

### 4. Make clippy pass or explicitly scope lint policy

- Files: `Cargo.toml`, codebase-wide cleanup
- Action: either fix current warnings or define accepted allowances narrowly
- Success criteria: CI can fail on meaningful regressions

## Phase 5: Release and Maintenance Hygiene

Target: 1-2 days

### 1. Commit `Cargo.lock` for the application

- Files: `Cargo.lock`, `.gitignore`
- Action: stop ignoring `Cargo.lock`
- Success criteria: reproducible builds and safer `cargo install --locked`

### 2. Add CI

- Files: `.github/workflows/ci.yml`
- Action:
  - `cargo fmt --check`
  - `cargo clippy --all-targets`
  - `cargo test`
- Success criteria: every PR is gated

### 3. Update README to match reality

- File: `README.md`
- Action:
  - list all current tabs or intentionally trim them
  - document privilege model
  - document tested Slackware versions
  - recommend `cargo install --locked`
- Success criteria: README matches shipped behavior

## Defer or Re-Scope

These areas appear incomplete and should either be finished or temporarily hidden:

- `NetworkMode::EditInterface` in `src/components/network.rs`
- several dead message variants in `src/app.rs`
- unused async traits and fields across components, starting in `src/components/mod.rs`

## Recommended Execution Order

1. Phase 0
2. Phase 1
3. Phase 2
4. Phase 4
5. Phase 3
6. Phase 5

Reason: responsiveness and safety should be fixed before expanding Slackware-specific workflow fidelity.

## Best Return First

If time is tight, do these first:

1. Fix the config-editor panic and add panic-hook cleanup
2. Remove render-time process execution
3. Move updater, sbotools, package, service, and network actions into background tasks
4. Centralize command execution
5. Add mirror rewrite tests and render smoke tests
