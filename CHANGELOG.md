# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v1.1.1] - 2026-07-07

### Added

- Sharded maps implementation for lowering lock time for PeerManager and IPTable

### Changed

- Use sharded maps as the new strategy for concurrency rather than pure single mutexes

## [v1.1.0] - 2026-07-06

This release adds [GNet](https://github.com/panjf2000/gnet/v2) as a new engine for the UDP server, enabling multicore for processing packets, as well as adding the golang's pprof tool suppor for profiling.

This release also comes with minor changes and fixes for the normal UDP server and server monitor.

### Added

- New refactored GNet engine for the UDP server
- Added multicore support when using GNet engine
- Added golang's pprof tool support for performance and resource profiling

### Changed

- Minor refactoring was made on the original UDP server
- Updated monitor to support GNet engine

### Fixes

- Fixed bugs on auth mechanism (peer.Chalenge not beign set on new sessions)

## [v1.0.0] - 2026-07-02

The first release of the server

### Added

- Full compatibility with the original switch-lan-play server
- Implemented authentication by HTTP, JSON and simple auth (username:password)

