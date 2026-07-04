# Free Fire 1.70 — TCP / Gateway protocol (notification channel)

Reverse-engineered from `libil2cpp.decrypted.so`, class `GCommon::TCPSession`
(+ `TCPMsgPacket`, `TCPParameters`). This is the persistent, bidirectional TCP
channel the client keeps open after login so the server can push messages at any
time (e.g. kick/ban). The client connects to the address returned as
`notification_channel` in `GetLoginData`/`LoginRes` — i.e. **host:10300**.

Source functions:
- `TCPSession::CreateAes` @ 0x32fa648 — static key/IV + cipher config
- `TCPSession::Encrypt` @ 0x32fac5c / `Decrypt` @ 0x32fd3e4
- `TCPMsgPacket::Serialize` @ 0x32fa098 — outbound (client→server) framing
- `TCPSession::OnRecvDataThread` @ 0x32fbb98 — inbound (server→client) parsing
- `TCPSession::OnConnected` @ 0x32fb8c0 — handshake
- `TCPSession::OnKeepAliveThread` @ 0x32fcf08 — heartbeat
- `TCPParameters::cctor` @ 0x32fa19c — constants

## Encryption

**AES-128-CBC, PKCS7 padding** (`Mode=CBC`, `KeySize=128`, `BlockSize=128`,
`Padding=PKCS7`). Static key + IV, identical every run:

```
Key (16) = 18 60 A2 34 CF 6F 8B 85 3D F6 90 34 4B E7 91 DD
IV  (16) = 02 F5 98 9E C0 26 B2 DF B2 44 21 AD 1A 9F 90 8D
```

Encryption is **one-directional** — only the frame payload, and only
client -> server:
- **client -> server**: the payload is AES(key,iv) ciphertext (gated by
  `COW.GameVarDef.TCPTokenEncrypt`, ON in this build). The server DECRYPTS it.
  The frame length is the length of the *ciphertext*.
- **server -> client**: the payload is **PLAINTEXT** protobuf. The client's
  receive path (`GCommon::ServiceClient::HandleRecvPacket`) deserializes the
  payload directly with `ProtoBuf.TypeModel.Deserialize` and NEVER decrypts
  (`TCPSession::Decrypt` has no code xref). So the server must NOT encrypt its
  responses — doing so makes the client fail to parse and silently drop them.

The frame header is always plaintext.

## Frame formats (ASYMMETRIC — this is the tricky part)

The header differs by direction, and the `region` byte only exists client→server.
All multi-byte integers are **big-endian** (network byte order,
`IPAddress.HostToNetworkOrder`). Multiple frames may be concatenated in one TCP
segment; a partial trailing frame is buffered until the rest arrives.

### Client → Server  (`TCPMsgPacket::Serialize`)
```
+------+--------+-----------------+---------------------+
| Cmd  | Region | Length (u32 BE) | Payload (Length B)  |
| u8   |  u8    |    4 bytes      |  = AES(plaintext)   |
+------+--------+-----------------+---------------------+
```
- If there is no payload (heartbeat, Cmd=2), Serialize writes **only** `Cmd` +
  `Region` (2 bytes) — no Length, no payload.
- So server-side parse: read `Cmd`(1) + `Region`(1); if `Cmd == 2` the frame ends
  there, otherwise read `Length`(4 BE) then `Length` payload bytes.

### Server → Client  (`OnRecvDataThread`)
```
+------+-----------------+---------------------+
| Cmd  | Length (u32 BE) | Payload (Length B)  |     (NO region byte!)
| u8   |    4 bytes      |  = AES(plaintext)   |
+------+-----------------+---------------------+
```
- Special case: **`Cmd == 2` is a single byte `0x02`** with no length/payload —
  the "server init ack" (see handshake).
- The client needs ≥5 bytes after the Cmd to read a full header; otherwise it
  rebuffers and waits for more.

## Reserved command bytes (`Cmd`)

| Cmd | Meaning |
|-----|---------|
| 1   | Session/auth. Client's first packet: payload = `AES(UTF8(login_token))`. Also used internally by the client as the "connected" app signal (len 0). |
| 2   | Heartbeat (client→server, every 8 s) / server-init-ack (server→client, single byte `0x02`). |
| 11  | `KICK_BY_SERVER_MSG_CMD` — server→client kick/ban. Payload = a protobuf kick message; the client deserializes it to a `DisconnectedReason` and closes the session. |
| other | Application messages: payload = `AES(protobuf)`. |

`Region` is populated from the account/region context — observed value `1`
against the live 1.70.1 client (not `0`; `TCPParameters.DEFAULT_REGION` is the
fallback only). It is an opaque passthrough byte for framing purposes.

## Handshake

1. After login (HTTP), the client reads `notification_channel = host:10300` from
   `GetLoginData` and opens a TCP connection there.
2. On connect the client spawns recv/send/heartbeat threads and immediately sends:
   `Cmd=1, Region=0, Payload=AES(UTF8(login_token))`.
   The token is the same login token issued over HTTP — the server should decrypt
   it and resolve the account (same store as `player.getByToken`).
3. The server replies with a single byte **`0x02`** ("server init back"). This
   sets the client's `m_ServerConfirmed`, starts its heartbeat, and raises the
   app-level "connected" event.
4. Steady state:
   - Client sends a heartbeat `Cmd=2` (bytes `02 00`) every
     `KEEP_ALIVE_INTERVAL_TIME = 8 s`.
   - The client's liveness timer resets on **any** received data. If nothing is
     received for `DEFAULT_DEACTIVE_TIME = 180 s` it declares a disconnect, so the
     server should reply to heartbeats (e.g. echo a single `0x02`) or otherwise
     send traffic well within 180 s.
5. The server may push a message at any time: `Cmd + Length(BE) + AES(protobuf)`.
   To kick/ban: `Cmd=11 + Length + AES(kick protobuf)`.

## Constants (`TCPParameters`)

| Name | Value |
|------|-------|
| `KEEP_ALIVE_INTERVAL_TIME` | 8.0 s |
| `DEFAULT_DEACTIVE_TIME` | 180.0 s (foreground disconnect timeout) |
| `BACKGROUND_DEACTIVE_TIME` | 20.0 s |
| `JOIN_TIMEOUT` | 100 |
| `KICK_BY_SERVER_MSG_CMD` | 11 |
| `TCP_MTU` (recv/send buffer) | 0x2000 (8192) |

## `TCPMsgPacket` field offsets (IL2CPP, for reference)

`+16` Cmd (u8) · `+17` Region (u8) · `+20` Length (i32) · `+24` Data (byte[]).

## Validated against the live client (1.70.1, 2026-07-01)

Captured on a throwaway listener on :10300 while the real client logged in as a
guest. Every prediction held:

```
CONN from 192.168.15.17:53525
01 01 00000040 <64B>   cmd=1 region=1 len=64
   AES-decrypt -> "f03d2e6243e5e600a935352ea6970ac35b751dc539f76f43"  (48 hex)
<- server replied single 0x02 ; client accepted it and continued
03 01 00000010 <16B>   cmd=3 region=1 len=16
   AES-decrypt -> protobuf 08 07 12 07 12 05 "pt-br"
02 01                  cmd=2 heartbeat  @ t
02 01                  cmd=2 heartbeat  @ t+8s
```

Confirmed:
- AES-128-CBC key/IV above decrypt both the token and an app protobuf cleanly
  (valid PKCS7) — key/IV are correct.
- `cmd=1` payload is the UTF-8 **session token issued by `MajorLoginRes.token`**
  (NOT the client's Garena `login_token`). Auth = decrypt cmd=1 →
  `player.getByToken(token)` → account.
- Framing `cmd│region│len(BE)│payload` is exact; heartbeat is the 2-byte
  `02 <region>` every 8 s.
- Replying to the auth packet with a single byte `0x02` is accepted as the
  "server init ack" — the client proceeds and starts heartbeating.

