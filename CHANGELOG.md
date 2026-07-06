# Changelog


All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- New refactored GNet engine for the UDP server
- Added multicore support when using GNet engine
- Added golang's pprof tool support for performance and resource profiling

### Changed

- Minor refactoring was made on the original UDP server
- Updated monitor to support GNet engine

### Fixes

- Fixed bugs on auth mechanism (peer.Chalenge not beign set on new sessions)

## [1.0.0] - 2024-07-02

The first release of the server

### Added

- Full compatibility with the original switch-lan-play server
- Implemented authentication by HTTP, JSON and simple auth (username:password)

