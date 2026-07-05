# AT Message Forward

Small Go program for reading SMS messages from an AT-capable cellular modem over a USB serial port and forwarding them to Telegram.

It works with essentially any modem that speaks the standard 3GPP AT command set (SIMCom, Quectel, EigenComm/Air780E, Fibocom, u-blox, and similar) — the initialization sequence and SMS handling use only standard AT commands, not vendor-specific ones. The binary is self-contained — no Python or Node.js runtime needed.

It can:

- configure the serial port with `go.bug.st/serial`
- initialize SMS text-mode push with standard AT commands through `github.com/warthog618/modem/at`
- print AT command responses and raw SMS-related modem indications
- parse `+CMT` text and PDU SMS events
- forward parsed SMS messages to Telegram and expose Telegram inline-keyboard modem controls
- keep Telegram polling alive across serial disconnects and retry serial reconnects with bounded backoff

## Quick test

Create `config.json` in the current working directory:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

```sh
go run ./cmd/atmsgfwd forward
```

Then send an SMS to the SIM card. You should see raw modem lines like:

```text
+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"
Your code is 123456
```

Some modems report SMS in PDU mode instead. This has been tested with output like:

```text
+CMT:,24
07915892200417F5240BA14180889969F100006260918052200005E8329BFD06
```

The program decodes GSM 7-bit, UCS2, and 8-bit SMS PDU payloads.

## Libraries

The low-level serial work is delegated to `go.bug.st/serial`, so the program does not shell out to `stty` or open `/dev/tty*` manually. AT command request/response handling and unsolicited result code dispatch are handled by `github.com/warthog618/modem/at`.

The SMS PDU decoder remains local because it is small and follows the standard 3GPP TS 23.040 PDU format rather than depending on a library.

## Serial message format

The init sequence uses standard 3GPP AT commands:

```text
AT          (bare probe / link check)
ATE0        (echo off)
AT+CPIN?    (wait for SIM READY)
AT+CSQ      (signal quality, for logging)
AT+CMGF=1   (SMS text mode)
AT+CNMI=2,2,0,0,0
```

`AT+CNMI=2,2,0,0,0` asks the module to push new SMS messages directly to the serial port. In text mode, the received-message format is:

```text
+CMT: "<oa>",[<alpha>],<scts>[,<tooa>,<fo>,<pid>,<dcs>,<sca>,<tosca>,<length>]
<data>
```

Many modems default to PDU mode (`AT+CMGF=0`). In PDU mode, the same push notification is:

```text
+CMT: [<alpha>],<length>
<pdu>
```

For PDU mode, `<length>` is the TPDU length only. It does not include the SMS center address at the start of the PDU. The parser checks that length before decoding, so a shifted or partial serial read is rejected instead of producing a wrong SMS.

> Note on compatibility: this forwarder only auto-forwards messages that arrive as `+CMT:` (full SMS body pushed directly, i.e. `AT+CNMI=...,2,...`). If your modem only supports index-only notifications (`+CMTI:`), it will store the SMS on the SIM without pushing the body, and you would need to read it with `AT+CMGR=<index>`. Confirm your modem accepts `AT+CNMI=2,2,0,0,0`.

Reference AT command docs (OpenLuat, for the Air780E but broadly applicable):

- https://docs.openluat.com/air780e/at/app/Command_List/SMS/CMGF/
- https://docs.openluat.com/air780e/at/app/Command_List/SMS/CNMI/
- https://docs.openluat.com/air780e/at/app/Command_List/SMS/PDU%E7%9F%AD%E4%BF%A1%E7%BC%96%E7%A0%81%E6%A0%BC%E5%BC%8F%E4%BB%8B%E7%BB%8D/

To listen without sending the init AT commands:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "init_modem": false
}
```

## Port discovery

The program gets the general serial port list from `go.bug.st/serial`. On Linux, it also prefers stable `/dev/serial/by-id/*` entries and checks `/sys/class/tty` USB metadata. Ports whose USB names match known cellular-modem vendors (SIMCom, Quectel, EigenComm/Air780E, Fibocom, u-blox, and others) are ranked above generic serial ports, and lower data interfaces are preferred over `log` or `ppp` style interfaces.

List candidates:

```sh
go run ./cmd/atmsgfwd ports
```

Run with automatic discovery (leave `port` empty in `config.json`):

```sh
go run ./cmd/atmsgfwd forward
```

For long-running Linux deployment, prefer the stable symlink shown by `ports` when available, and set `port` in `config.json` to that stable symlink path.

## macOS / USB transport (no serial port)

Some modems do not expose a serial port on macOS. For example a Quectel
EC25-style module in ECM mode presents its AT command port as a
**vendor-specific USB interface (class 0xFF)**, which macOS has no driver for —
so no `/dev/cu.*` node is ever created (only its CDC-ECM network interface is
recognized). The default serial transport has nothing to open.

For these, build with the `usb` transport, which talks to the AT interface
directly over USB bulk endpoints using libusb.

Install libusb and build:

```sh
brew install libusb                 # macOS
# Debian/Ubuntu: sudo apt install libusb-1.0-0-dev
make buildUSB                       # or: CGO_ENABLED=1 go build -tags usb -o atmsgfwd ./cmd/atmsgfwd
```

Discover which USB interface answers AT:

```sh
./atmsgfwd ports
# ...
# usb candidate 2c7c:0125 "BAIWANG" "Baiwang" -> AT ok on if2
```

Enable it in `config.json`:

```json
{
  "transport": "usb",
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

By default the USB transport scans all connected devices for a vendor-specific
interface that answers `AT`. You can pin the device and interface if needed:

```json
{
  "transport": "usb",
  "usb_vendor": "2c7c",
  "usb_product": "0125",
  "usb_interface": 2,
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

Notes:

- The plain build (`go build`, `CGO_ENABLED=0`) does not include the USB
  transport; `transport: "usb"` then returns a clear "not built in" error.
- On **Linux** you usually do not need this: the kernel `option` driver creates
  `/dev/ttyUSB*` for the same modem, so the default serial transport works and
  no libusb/CGO is required.
- The USB transport does not need the modem's network interface; you can leave
  the ECM/RNDIS network card disabled.

## Telegram Control

Telegram is the program's push and control surface. `telegram_token` and `telegram_chat` are required for `go run ./cmd/atmsgfwd forward`; `telegram_chat` must be an int64 chat ID and is also the only authorized chat allowed to use the bot controls.

Configuration is read only from `config.json` in the current working directory. Missing files use built-in defaults, and empty string or zero number values in JSON fall back to defaults.

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "baud": 115200,
  "init_modem": true,
  "sim_ready_timeout_seconds": 120,
  "telegram_raw": false,
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

```sh
go run ./cmd/atmsgfwd forward
```

Open the bot chat and send `/start` or `/menu` to show the inline keyboard. The bot uses long polling and deletes any existing webhook before polling.

The menu provides status queries, SMS history queries, device controls, and help. Button-triggered AT commands are serialized before they are sent to the active modem session. If the USB serial device disconnects, Telegram polling stays active, the serial session is retried with bounded backoff, and automatic discovery is re-run when `port` is empty.

SMS pushes and watchdog alerts are queued before Telegram API calls, so slow Telegram HTTP requests do not directly block modem event handling. Optional raw-line forwarding uses a smaller best-effort queue and may drop raw lines under sustained congestion.

Available controls include:

- status summary, signal quality, network registration, operator, SIM status, and module information
- unread SMS, all SMS, and SMS storage queries
- current function mode, re-enable SMS push, and reset with confirmation

To forward every raw line as well:

```json
{
  "port": "/dev/cu.usbmodem0000000000013",
  "telegram_raw": true,
  "telegram_token": "123456:abc...",
  "telegram_chat": "123456789"
}
```

## Build

```sh
go build -o atmsgfwd ./cmd/atmsgfwd
```

The resulting binary does not need Python or Node.js.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
