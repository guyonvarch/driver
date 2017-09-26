# Change Log

## [UNRELEASED]

### Changed

- Set default address for Senso to 169.254.1.10 for backwards-compatibility

## [0.3.7] - 2017-09-25

### Changed

- Make Electron an optional dependency for 'headless' mode

## [0.3.6] - 2017-09-21

### Changed

- New SSL certs, obfuscated for longevity
- Linux only: Autoconnect to mDNS-discovered addresses

## [0.3.5] - 2017-09-17

### Changed

- Disable all automatic connections to mDNS-discovered addresses

## [0.3.4] - 2017-08-09

### Fixed

- Changes to build process to improve stability

## [0.3.3] - 2017-07-23

### Changed

- Use new SSL certificate for localhost.dividat.com alias

## [0.3.2] - 2017-06-27

### Added

- Update mDNS listener when new network interfaces appear

## [0.3.1] - 2017-06-20

### Added

- Release channels for auto-update functionality (Windows)

## [0.3.0] - 2017-06-16

### Added

- Support for zeroconf detection of Sensos on local network

### Changed

- Rotate logs to reduce disk use
- Check for updates hourly instead of half-daily (Windows)
- Remove filtering logic and reduce driver to a TCP <-> WebSocket bridge
- Simplify persistentConnection module by making returned connection an EventEmitter

### Removed

- TCP heartbeat in persistentConnection

## [0.2.2] - 2017-03-04

### Added

- Add installer and auto-update functionality for Windows

## [0.2.1] - 2017-02-02

### Changed

- Change default address for Senso to a link-local address
- Use new Dividat icons

## [0.2.0] - 2016-12.23

### Added

- SSL encryption from driver to clients

## [v0.1.0] - 2016-12-22
