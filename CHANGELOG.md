# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0] - 2026-04-09

### Added

- **pkg/config**: Parse `rancid.conf` into `config.Config` (env-var style KEY=value format, including BASEDIR, LIST_OF_GROUPS, FILTER_PWDS, FILTER_OSC, PAR_COUNT, MAILDOMAIN, MAILHEADERS, MAILOPTS, SENDMAIL)
- **pkg/config**: Parse `router.db` into `[]config.Device` (hostname:type:status format, comments and blank lines ignored)
- **pkg/config**: Parse `.cloginrc` into `config.CredStore` with first-match-wins glob lookup (add user/password/enablepassword/method directives)
- **pkg/devicetype**: Load `rancid.types.base` and `rancid.types.conf` with base-type protection (conf cannot override base types), alias resolution with cycle detection
- **pkg/git**: Git subprocess wrapper — `Init`, `Add`, `Commit`, `Diff` (no libgit2 dependency)
- **pkg/par**: Parallel worker pool bounded by `PAR_COUNT`, returns indexed results
- **pkg/collect**: `FallbackCollector` delegates per-device collection to the original Perl `rancid` binary — non-zero exit treated as `StatusFailed`, not a hard error
- **pkg/notify**: `BuildMessage` and `SendDiff` for email diff notifications via sendmail subprocess, with Precedence: bulk headers and MAILHEADERS override support
- **cmd/rancid-cvs**: Initialize git repos and per-group directory structure under BASEDIR (replaces `bin/rancid-cvs`)
- **cmd/control-rancid**: Per-group orchestrator — reads router.db, runs parallel collection, commits to git, emails diffs (replaces `bin/control_rancid`)
- **cmd/rancid-run**: Cron entry point — iterates LIST_OF_GROUPS, invokes control-rancid per group with logging (replaces `bin/rancid-run`)