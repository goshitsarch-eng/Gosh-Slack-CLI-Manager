# Changelog

All notable changes to `slackware-cli-manager` will be documented in this file.

The format follows Keep a Changelog conventions and uses semantic-style release headings where practical.

## [Unreleased]

No unreleased changes recorded yet.

## [0.1.0] - 2026-03-13

### Added

- Initial Slackware administration TUI covering updates, SlackBuild workflows, service management, logs, kernel management, backups, disks, networking, configuration editing, cron inspection, and settings.

### Changed

- Hardened the application architecture so long-running system operations run in the background instead of blocking the UI.
- Added panic-safe terminal restoration, atomic config writes for supported paths, safer mirror updates, stronger backup permissions, and root-gated mutation behavior.
- Added CI and repository hygiene improvements including lockfile tracking, README updates, and stricter test and lint coverage.
- Rebuilt the full terminal interface around a cohesive Ratatui visual system with shared colors, panel styling, status chips, and navigation chrome.
- Redesigned every major screen to use clearer header, content, inspector, and status regions instead of flat utility layouts.
- Standardized runtime badges, inspector panels, and action messaging across updater, package search, package browser, services, network, logs, backup, disks, settings, cron, mirrors, kernel management, config editing, user setup, sysinfo, and sbotools.
- Improved visual hierarchy for dense operational screens so long-running tasks, selected records, and high-risk actions are easier to track at a glance.

### Notes

- Hardening baseline landed in commit `a237c6d` (`Harden app architecture and release hygiene`).
- The Ratatui redesign landed in commit `bd89dec` (`Revamp TUI with cohesive Ratatui redesign`).
- Release validation completed with:
  - `cargo fmt --check`
  - `cargo test --all-targets --all-features`
  - `cargo clippy --all-targets --all-features -- -D warnings`
