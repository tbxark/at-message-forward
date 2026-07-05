# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go CLI that reads SMS messages pushed from an AT-capable cellular modem (SIMCom, Quectel, EigenComm/Air780E, etc.) over a USB serial port and forwards them to Telegram. It uses only standard 3GPP AT commands, so it is not tied to a specific vendor. The binary is self-contained — no Python/Node runtime needed.

## Commands

```sh
go build -o atmsgfwd ./cmd/atmsgfwd    # build (pure Go, CGO_ENABLED=0, serial transport only)
make buildUSB                          # build with libusb USB transport (CGO + -tags usb)
go test ./...                          # run all tests
go test ./internal/sms/                # test a single package
go test ./internal/sms/ -run TestDecodePDU -v   # run a single test

go run ./cmd/atmsgfwd ports            # list/rank serial port candidates
go run ./cmd/atmsgfwd forward          # read config.json, listen + forward
```

Two transports (config `transport`): `serial` (default, `go.bug.st/serial`) and
`usb` (libusb bulk endpoints, for modems with no serial port — e.g. Quectel
EC25 on macOS whose AT port is a vendor-specific USB class). The `usb` transport
is behind build tag `usb` + CGO; the default build ships a stub that errors
clearly. Verify USB changes with `CGO_ENABLED=1 go build -tags usb ./...`.

There is no lint config in the repo; use `go vet ./...` and `gofmt`.

## Architecture

Data flows in one direction through a fan-out of channels:

```
telegrambot.Service + polling ──┐
                                 ↓
serialport.Open → modem.AT (warthog618/modem/at) → +CMT indication handler
                                                         ↓
                              rawLines chan ──┐    events chan (sms.Event)
                                              ↓          ↓
                         forwarder session loop enqueues Telegram sends
                                              ↓
                              telegramSender worker serializes API calls
```

- **cmd/** — Cobra root with two subcommands (`forward`, `ports`). `forward` reads runtime configuration from `config.json` by default and accepts an optional config path.
- **internal/config/** — `Config` struct + `Default()` + JSON loading from `config.json`. Missing files use defaults; empty string and zero number values fall back to defaults.
- **internal/forwarder/** — `Run()` validates config, starts Telegram once, starts a watchdog, then runs a reconnect loop. `runModemSession` dispatches on `cfg.Transport` to `runSerialSession` (autodetect/open serial) or `runUSBSession` (`usbserial.Open`); both feed a shared `runOpenedModemSession(io.ReadWriteCloser)` that creates `modem.AT`, registers indication channels, initializes the modem when enabled, swaps the active AT executor, and selects over `rawLines`, `events`, `ctx.Done()`, and modem-closed. Telegram sends are queued so HTTP latency does not block modem event handling.
- **internal/usbserial/** — libusb (gousb) transport implementing `io.ReadWriteCloser` over a modem's vendor-specific AT interface bulk endpoints. `Open` finds the device (optional VID/PID), claims config 1 **without** auto-detaching kernel drivers (so it coexists with the CDC-ECM network interface on macOS), and auto-probes 0xFF interfaces for one that answers `AT`. Real impl is `//go:build usb` (+CGO/libusb); a `!usb` stub returns `ErrNotSupported`.
- **internal/modem/** — Wraps `warthog618/modem/at`. `NewAT` registers `+CMT:`/`+CMTI:`/`+CDS:` unsolicited-result-code handlers; `+CMT:` parses into an `sms.Event`. `Init` sends the generic 3GPP AT init sequence (`E0`, `+CPIN?`, `+CSQ`, `+CMGF=1`, `+CNMI=2,2,0,0,0`).
- **internal/sms/** — Self-contained SMS parser. `ParseCMTIndication` distinguishes text-mode (`+CMT: "<oa>",...`) from PDU-mode (`+CMT: [<alpha>],<length>`) headers. `DecodePDU` validates TPDU length, parses SMS-DELIVER and TP-VPF-aware SMS-SUBMIT layouts, and decodes GSM 7-bit / UCS2 / 8-bit user data. This decoder is intentionally local (not a library) because it follows the standard 3GPP `+CMT` PDU format.
- **internal/telegrambot/** — Telegram service, long polling, inline keyboard menus, command formatting, and serialized AT command execution through the `Executor` interface. The forwarder supplies a reconnectable executor that returns a clear not-connected error when no serial session is active.
- **internal/serialport/** — Port discovery and scoring. `Candidates()` ranks ports; known cellular-modem vendor names (`modemNameMarkers`: SIMCom/Quectel/EigenComm-Air780E/Fibocom/u-blox/…), USB interface metadata (Linux `/sys/class/tty`), and stable `/dev/serial/by-id/*` symlinks score higher. `if03`/lower data interfaces beat `log`/`ppp`.

## Key behaviors to preserve

- **PDU length validation**: `DecodePDU` rejects a PDU when the actual TPDU length (after skipping the SMSC address) doesn't match the `<length>` from the `+CMT:` header. This guards against shifted/partial serial reads producing a wrong SMS. Don't remove this check.
- **Non-blocking channel sends**: `emitRawLines`/`emitSMSEvent` use `select`/`default` and drop (with a stderr warning) rather than block when the channel buffer is full.
- **Queued Telegram sends**: forwarder queues SMS/watchdog alerts and best-effort raw-line sends before Telegram HTTP calls. Keep modem indication handling decoupled from Telegram latency.
- **Reconnect behavior**: serial disconnect/open/init errors are retryable until context cancellation. For empty `port`, autodetect is re-run on each retry so a re-enumerated USB device can be found.
- Many modems default to PDU SMS mode (`AT+CMGF=0`); init switches to text mode (`CMGF=1`) but a modem can still push PDU-mode `+CMT`, so both paths must keep working.

## Reference

AT command docs for `CMGF`, `CNMI`, and PDU encoding (OpenLuat, broadly applicable to 3GPP AT modems) are linked in README.md.
