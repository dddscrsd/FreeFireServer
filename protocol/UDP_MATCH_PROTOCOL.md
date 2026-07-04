# Free Fire 1.70 — UDP match (game server) protocol

Reverse-engineered from `libil2cpp.decrypted.so`: `GCommon::UDPSession`,
`GCommon::UDPMsgPacket` (Serialize/Unserialize/IsReliable), `HandleRecv`,
`GCommon::S2C_Hello_Res`, `GCommon::UDPChecksum::CRC7`,
`GCommon::NetworkCryptologyUtil` (TEA). Crypto also mirrored in
`original_to_read/fast_crypto.c` (from the prior Python emulator) — VERIFIED to
match the binary.

The client connects here (UDP) after matchmaking, using the `server_addr` from
`MatchmakingSussNtf` (i.e. the game-server host:port, `:10100` in our config).
The TEA/gzip key is `UDPParameters.SECRET_KEY = HexStringToByte(SussNtf.secret)`
— i.e. the `secret` we hand out in the SussNtf.

## Packet framing

All integers are little-endian. `msg_key` is a fixed byte **`108` (0x6C)** — the
client drops any packet whose first byte ≠ 108 ("invalid message header, msg key").

```
Unreliable (8-byte header):
  msg_key(1)=0x6C | crc7(1) | send_option(1) | cmd(u16) | flags(1) | length(u16) | payload

Reliable (12-byte header):
  msg_key(1)=0x6C | crc7(1) | send_option(1) | cmd(u16) | seq_id(u16) | order_id(u16) | flags(1) | length(u16) | payload
```

`length` is the length of `payload` as it appears on the wire (i.e. AFTER
encryption/compression).

### CRC7
Seed 0, over the packet bytes from **offset 2** (the `send_option` byte) through
the end of `payload` — i.e. everything except `msg_key` and `crc7`. 7-bit
(masked `& 0x7F`), table polynomial `0x09` (see `fast_crypto.c` /
`UDPChecksum::CRC7`). On receive the client recomputes `CRC7(0, buf[2:])` and
drops on mismatch.

### send_option  (drives reliability + routing; see `UDPMsgPacket::IsReliable`)
`IsReliable` = `cmd != 2` AND `send_option ∈ {1, 2, 5}`.

| send_option | meaning |
|-------------|---------|
| 0 | unreliable |
| 1 | **connection/HELLO response** — client parses payload as `S2C_Hello_Res` (special-cased in `HandleRecv`), reliable |
| 2 | normal reliable app packet — enqueued to the message handlers |
| 3 | **kick** ("kicked by server" → session closed) |
| 5 | reliable variant |

`cmd == 2` is always unreliable and is the **ACK / keepalive** channel (below).

### flags
- bit 0 (`&1`) = payload is **TEA-encrypted** (key = `SECRET_KEY`)
- bit 1 (`&2`) = payload is **gzip-compressed** (also keyed by `SECRET_KEY`)
- Receive order: decompress (if `&2`) THEN decrypt (if `&1`). Send order is the
  reverse. We only ever send `flags=1` (encrypted) or `flags=0` (plain); no gzip.

### TEA (modified CBC, from `fast_crypto.c`, matches `NetworkCryptologyUtil`)
16-round TEA, `delta=0x9E3779B9`, 16-byte key. Modified QQ/Tencent CBC with a
1-byte padding-count header, 2 salt bytes, and 7 trailing zero bytes (used to
verify integrity on decrypt). See `c_tea_encrypt`/`c_tea_decrypt`.

## Reliability (seq / order / ack)
Reliable packets carry `seq_id` (per-packet, increments) and `order_id`
(delivery order). On receiving ANY reliable packet, the peer immediately sends an
**ACK**: `cmd=2`, `send_option=0` (unreliable), payload = `seq_id(u16) | ack_bits(u32)`.
`ack_bits` is a bitmask of recently-received sequence ids relative to `seq_id`
(`GenAckBits`). The sender resends unacked reliable packets. So our server MUST
ACK the client's reliable HELLO/JOIN, or the client resends them forever.
(`cmd=2` inbound from the client is an ACK → `HandleAck(seq, ack_bits)`.)

## Handshake / match-join flow

1. Client opens the UDP socket and immediately sends **HELLO**:
   `cmd=1, send_option=1, payload=empty` (reliable, empty → `flags=0`).
2. Server → **`cmd=1, send_option=1`**, payload = `S2C_Hello_Res`:
   ```
   keyLen(i32) | SessionKey(keyLen bytes, UTF-8) | OrderID(u16) | RequiredID(u16) | EnableFastProto(u8)
   ```
   Client stores `SessionKey`, sets `ENABLE_FAST_PROTO`, marks the connection
   established. Set `EnableFastProto=0` (keep standard protobuf).
   Server must also ACK the client's HELLO (cmd=2 ack).
3. Client → **JOIN_MATCH**: `cmd=100`, reliable (send_option=2), payload =
   `MatchmakingJoinReq`-equivalent (TODO: structure).
4. Server → **`cmd=100`** reply (reliable send_option=2, encrypted) using the
   join-response class (TODO: class name/structure). If framing + reliability are
   correct the client accepts the join (it will NOT leave the loading screen yet).
5. Server → **PLAYER_JOIN** `cmd=101` (without waiting for a client packet): info
   about a joining player. The player's account info must be our player, and
   either its account_id == our player's, or the player/entity id ∈ {0,1}.

## Command ids (`message.OPNCEEBBMJH` enum; obfuscated)
Server→client handler registrations live in
`COW::GamePlay::KPDMJKOEHEE::DPLMGOJKKCM` (@0x1926ea0) as
`NetworkMessageDispatcher::RegisterHandler<T>(cmd, handler)` calls — each gives
the cmd, the message class `T`, and the handler method. Client→server cmds
(like 440) have no client-side handler.

| cmd | dir | name / class | notes |
|-----|-----|--------------|-------|
| 1   | both | HELLO / `S2C_Hello_Res` | connection handshake (send_option 1) |
| 2   | both | ACK / keepalive | unreliable `[seq u16][ackbits u32]` |
| 3   | both | PING | unreliable 4-byte payload, ~1 Hz; both sides send |
| 5   | c→s | reconnect hello | carries the `SessionKey` string |
| 100 | s→c | JOIN_MATCH result / `message::LGIGCGIDOKP` | handler `KPDMJKOEHEE::GLPOGGHMJAB`; fires `JOIN_MATCH` event |
| 101 | s→c | PLAYER_JOIN / `message::GKBDLJFGGMI` | handler `KPDMJKOEHEE::OFEBIEPAIKH` → `PlayerNetwork::PELEGEAEILN` / `EPPlayerInfo` |
| 103 | s→c | match end | seen in `HandleRecvPacket` replay path |
| 439 | c→s | RUDP_JOIN_MATCH_PREPARE | not observed (client skipped to POST) |
| 440 | c→s | RUDP_JOIN_MATCH_POST / `C2S_RUDP_JoinMatch_Req` | carries our `prepare_token` (a JWT) in the Token field |

### cmd 440 `C2S_RUDP_JoinMatch_Req` — extracting the prepare_token
The client echoes our `MatchmakingSussNtf.prepare_token` (a JWT) somewhere in the
cmd 440 payload. The reference emulator
(`original_to_read/udp.py:deserialize_join_match_req`) documents this nominal layout:
```
u64 UserID | u32 RoomID | u8 RoomType | u8 GameMode | u64 ServiceRoomID
| string Token (i32 len + UTF-8)  ← the prepare_token
| ...optional (u32 map_id, bool is_reconnect, ...)
```
**Those fixed offsets do NOT line up on the live 1.70 payload** (the reference
parser itself fell back to raw reads). Verified live: cmd 440 arrives encrypted
(`flags=1`), decrypted length ~845–850 B, and reading `Token` at offset 22 yields
nothing. So the match server instead **scans the payload for the JWT** by its
fixed header marker `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9` and reads the maximal
run of JWT chars (`match-server/message/joinreq.go:ExtractJWT`). This is
layout-independent and recovers the ~697 B token reliably. `ParseJoinMatchReq`
(the struct parse) is kept only as a secondary fallback; a missing token falls
back to the stub player (with a payload hex dump logged for diagnosis).

### Match message serialization
Match payloads are NOT protobuf. Each `message::*` is a
`GCommon::UDPClientMessageBase` subclass with its own `Serialize(FastBinaryWriter)`
/ `UnSerialize(FastBinaryReader)`. With `ENABLE_FAST_PROTO=false` (which our
`S2C_Hello_Res` selects) all ints are **fixed-width little-endian**
(`TryReadFixU32` etc.); strings are `int32 length + UTF-8 bytes`; lists are
`int16 count + elements`; bool/byte are 1 byte; Single is 4-byte float.

### cmd 100 `LGIGCGIDOKP` layout (join result) — VERIFIED accepted live
```
u32 result | u64 | u64 | u64 | i32 | u8 | u32 | u16 | u32 | u32
| PGKJDKIJGJD{ i16, DEACEIFBHJK{i32,i32,i32}, DEACEIFBHJK{i32,i32,i32}, f32, f32 }
| list<i32>(i16 count) | list<u32>(i16 count) | list<msg>(i16 count)
| u32 | u32 | bool x5 | u32 | bool x3
| EKHEMGJLHDC{ string, string, bool, list<byte>(i16 count) }
| bool
```
`result` is a `message::PHIFIIFMJKP` enum; **0 accepts the join**. Sending this
(all other fields zero/empty) as `cmd=100, send_option=2, flags=1` makes the
client ACK it and hold on the match-loading screen (no reconnect). The handler
just fires the `JOIN_MATCH` event, so exact field values only matter once we
want the match to actually start.

### cmd 101 `GKBDLJFGGMI` layout (PLAYER_JOIN) — VERIFIED accepted live
```
OGJKHJAFNHB DCAFPIJDJEL | u32 CEDJCPLOLNE | PKPAMKEDCDC IFHIJHODIKE | i32 | i32 | u8
```
`OGJKHJAFNHB` (player profile): `u64 MIJOCMKONAD(account id) | u64 | u32 IHAAMHPPLMG(entity id) | string name | DEACEIFBHJK(3xi32) | HJLNLDIMIMK(3xi16) | bool | u8 | u8 | list<JOLBCNEHKLJ>(i16) | u8 | u32 | u32 | u8 | u16 | u8 | u32 | u8 | bool | u32 | u16 | u16 | u32 | u32 | u8 | string | u64 DBBDCBDBDIG | u32 role | u16 | bool`.
`PKPAMKEDCDC` (cosmetics): mostly u32/u64/string/bool + six `list<u32>`; two of
those lists (MCPGFPHMOGM, GLHCLEMMLDN) are "read-ahead" (always 1 trailing u32).
Handler `OFEBIEPAIKH`: `Player::IsLocalPlayer(MIJOCMKONAD, IHAAMHPPLMG)` (account
match OR entity ∈ {0,1}) → `NFJPHMKKEBF::LONDNBHBPDO(...)` adds the player.

### Live join-flow result
`HELLO → S2C_Hello_Res → POST(440) → cmd 100 (result=0) → cmd 101 (account
10000001, entity 1) → cmd 130 (JoinMatchFinished)`: the client ACKs all,
**spawns the local player**, and streams **`cmd=1001` = player movement/transform**
(unreliable, ~continuous; first u32 = entity id).

### Leaving the loading screen — WORKING
The client **enters the match** (loading mask lifts) once the join sequence is
sent. The loading mask is gated **entirely client-side** (no extra server packet):
`UIInGameScene::CheckToCloseHUDMask` closes it when `m_PreloadLoaded` (set on
scene-load) `&& m_LocalPlayerAdded` (set by our cmd 101 local `ADD_PLAYER`);
`CloseHUDMask`'s stream sub-gate (`StreamerFacade.IsMainStreamFinished`) is set
unconditionally in `MatchGame::OnSceneLoaded`.

**Critical fix:** cmd 100 `LGIGCGIDOKP` must NOT be all-zero. Zeroed fields make
the client abort scene-init (before `UI_PRELOAD_LOADEDNTF` fires), so the mask
never lifts. Populating non-zero values (match id `DGCJANAMDBJ`, `ServerTime`
`HINHHMCOFLD` = unix now, a non-zero `MatchSeed` `FMMDBCOBGDM`, and the
`FEAMHOEGDAG` bool = true) fixes it — see `match-server/message/message.go`.
cmd 101 can be sent immediately after cmd 100 (no delay needed).

The cmd 101 player identity (account id + name) is now built from the
`prepare_token`: the TCP matchmaker signs an **HS256 JWT** (claims `aid`, `name`,
`region`, `mid`, and a `show` object with the player's selected cosmetics) with
the shared `MATCH_JWT_SECRET`; the client carries it in cmd 440's Token field;
the match server verifies it (`match-server/token/jwt.go`, same secret via env)
and builds PLAYER_JOIN from the claims. If the token is missing/invalid the
server falls back to the stub player so a manual join still works.

The JWT also carries the cosmetics (`show`: avatar/skin_color/head/banner/
clothes/slots), but those are not yet serialized into cmd 100/101 — the
`PKPAMKEDCDC`/`OGJKHJAFNHB` cosmetic field mapping is still unverified, so
populating them risks re-breaking the loading-mask gate. That mapping is the
next RE step; the data is already flowing to the match server.

## Key source functions
- `UDPMsgPacket::Serialize` @0x3302f54 / `Unserialize` @0x33028d8 — framing
- `UDPMsgPacket::IsReliable` @0x3302cf4
- `UDPSession::HandleRecv` @0x3307668 — msg_key/CRC7 checks, ack, hello, dispatch
- `UDPSession::OnConnect` @0x3305ac0 — sends the initial HELLO
- `UDPSession::Send` @0x3305f6c
- `S2C_Hello_Res::UnSerialize` @0x32f394c
- `UDPChecksum::CRC7` @0x3301cb4/0x3301d68/0x3301e6c
- `NetworkCryptologyUtil::TeaEncrypt/TeaDecrypt` @0x30d0e7c/0x30d039c

## Open items (next RE pass)
- Is the `S2C_Hello_Res` (send_option=1) reply encrypted (`flags=1`) or plain
  (`flags=0`)? (Hello is likely plain; app packets encrypted.)
- Exact meaning the client assigns to `S2C_Hello_Res.OrderID` / `RequiredID`
  (reliable-ordering init).
- `JOIN_MATCH` (100) request + response structures, and the reply class name.
- `PLAYER_JOIN` (101) structure + which fields set player/account id.
- `GenAckBits` / `HandleAck` exact bit layout (for correct resend suppression).

## Replication: cmd 500 (PRI) / 501 (GRI) / 118 (BindPRI) — VERIFIED

The match state is driven by a **replication** system, not typed messages. Two
raw-`FastBinaryReader` handlers in `COW::GamePlay::KPDMJKOEHEE` receive it:

| cmd | name | handler | routes to |
|-----|------|---------|-----------|
| 500 | PRI (Player Replication Info) | `DNCBMEDLPGP` @0x194e080 | per-entity pool (looped by RepID) |
| 501 | GRI (Game Replication Info)   | `PNJHGADAGFN` @0x194e2b4 | `MatchGame.m_GRIDataPool` (single) |
| 118 | BindPRI                        | (client)                | binds RepID → entity |

### Replication block (the TLV codec) — `ReplicationDataPool::SyncReplicationData` @0x33f4f5c
```
blockLen : u16 (LE)                 -- byte count of the tuples that follow
repeat while (pos - start) < blockLen:
  typeCode : u8      -- 0=i8 1=u8 2=i16 3=u16 4=i32 5=u32 6=bool 7=f32 8=u64 9=i64
  fieldId  : u8      -- SetData<T>(fieldId) key (must match the field's registered type)
  value    : per typeCode (fixed-width LE; ENABLE_FAST_PROTO must stay false)
```
Only changed fields are sent; order is free. A wrong typeCode calls a
different-typed `SetData<T>` and the client's typed handler won't fire.

- **cmd 501 (GRI)** payload = one block (no RepID prefix).
- **cmd 500 (PRI)** payload = repeated `[RepID u32][outerLen u16][block]` per entity
  (the entity wrapper adds an *outer* u16 length around the block's own `blockLen`).
  `RepID = 500 + (EntityGameID - 1001)` in the reference scheme, but the server
  chooses RepIDs and declares them via BindPRI.

### cmd 118 BindPRI — `[count u32]` then per entry `[RepID u32][EntityType u8][EntityGameID u32]`
`EntityType`: 1=Player, 3=Vehicle, 5=OilDrum. The local player must be the FIRST
`EntityType=1` entry — the client adopts that RepID as its own player.

### Transport (from the working reference emulator)
- BindPRI (118): reliable (send_option 2, seq/order) + `flags=1` (encrypted).
- PRI/GRI (500/501): **send_option 4, `flags=0` (plaintext), unreliable**, resent
  periodically (~0.15 s PRI loop). (Our first GRI test used so=2/flags=1; align to
  so=4/plaintext to match the client's replication path.)

### Contra Squad (game mode 15) GRI — `COW::GamePlay::JBCMHIAGMHA` (IsCSMode: mode 15/41/52)
CS state lives in JBCMHIAGMHA's 7-field GRI pool (`InitGRIData` @0x26b3e90). Field
ids == wire fieldIds:

| fieldId | type | meaning | handler |
|--------|------|---------|---------|
| 1 | u8  | maxRound (roundsToWin=(max+1)/2) | LOBNNPMPOJA @0x26b6694 |
| 2 | u8  | currentRound (0-based; getter +1) | CHCIJBJNJEJ @0x26b5110 |
| **3** | **u32** | **(ECSMatchPhase<<16) \| param** — MAIN GATE | LGBJACEELLK @0x26b54d0 |
| 4 | u8  | matchPoint (bool) | FLJEJOLIHBC @0x26b6720 |
| 5 | u32 | (EMatchDrawState<<16) \| timer | JIKLJNHJOAG @0x26b5a38 |
| 6 | u32 | airdrop sync id | FOOJLFAEAMC @0x26b7208 |

`ECSMatchPhase`: 0=Waiting 1=Prepare(buy/shop) 2=Fight 3=Post 4=Cutscene 5=Introduction.
`EMatchDrawState`: 0=NormalStart 1=HalfwayJoin 2=Draw 3=MatchEnd 4=CancelDraw.

### Movement gate (why the CS player is frozen) — NOT BattleStart (cmd 110, BR-only)
The local player is confined while `ECSMatchPhase` (GRI field 3 hi16) is `Waiting(0)`:
- `UILoadingController::CheckCanDestroy` @0x2678e20 keeps the intro/loading screen up
  until phase ≠ Waiting ("intro plays, never progresses").
- `JBCMHIAGMHA::ShowSpawnAreaFences` @0x26b4a3c keeps spawn colliders up while
  phase < Fight(2) — physically confining the player.
Advancing phase Waiting→Prepare (mask off + shop, via `JPBHIHNACHL` @0x26b5358)
→Fight(2) (fences drop) is the CS unlock. **Also required:** the local player's
**PRI (cmd 500) HP must be set** (VarID 0 cur_hp / VarID 1 max_hp, u16, default 200)
— an unsynced player is HP 0/0 (dead) and cannot walk (verified live: camera-look
works, locomotion blocked, HUD shows HP 0/0).

### PRI player field table (`build_pri_hp_block`; from OnUserDefineReplicationInfo @0x190676c)
VarID 0 cur_hp(u16/g3), 1 max_hp(u16/g3), 2 vest(u64/g8), 3 helmet(u64/g8),
4 itemOnHand(u64/g8), 5 isRescuing(u8/g1), 21 fireState(u8/g1), 6 curEP(u8/g1),
7 maxEP(u8/g1), 8 status(u32/g5), 39 (u16/g3), 9 sightingId(u32/g5),
10 killCount(u8/g1), 11 obCount(u8/g1), 12 buff(u32/g5), 13 camoHP(u8/g1),
14 likedCount(u8/g1), then 15–38 (defaults 0; 32 curCoin u16). Send in this order.

### Client → server CS
- cmd 585 (0x249) `LEHHDEPGAEH{List<u32> itemIds}` = CS shop purchase (when
  `GameModeSetting.EnableAskPurchaseCSItem`), sender `UIModelMatch::AskCSPurchase` @0x1ae6d58.

### Status
- Implemented + sent: cmd 501 GRI CS phase advance (init + Prepare→Fight). Verified
  on the wire (client ACKs). Movement still blocked — player is HP 0/0.
- **Next:** send cmd 118 BindPRI (local player first) + cmd 500 PRI (HP=200) so the
  player is alive; then re-verify movement + wire the CS round loop (phases/shop/scoreboard).

## Match state gate + cmd 110 BattleStart (the real CS unlock) — VERIFIED

The client silently DROPS all replication (cmd 500/501) unless the match is in a
"running" state. Gate: `KPDMJKOEHEE::MLCFKMGBBNH` @0x1932a9c = `CurrentMatch != null
&& BCMGLHIGJLL()`, where the state field is `NFJPHMKKEBF::ILGECLEFCCO` (int32 @+0x10):
```
BCMGLHIGJLL @0x1a2f908 = state==1 || NLCJBNKCGFK
NLCJBNKCGFK @0x1a2f984 = state==2 || DKMFGNPHFFE
DKMFGNPHFFE @0x1a2fa80 = state==3
```
Enum `LICPHHNNPPF`: 0=None (blocked), 1=Running, 2=WaitingForEnd, 3=ChickenDelay,
4=MatchEnd (latched, blocked). So replication is accepted only when state ∈ {1,2,3}.

**No server packet sets the state.** Sole setter `NFJPHMKKEBF::FGEKAPHFINE` @0x1a2eccc;
Running(1) is set CLIENT-SIDE by `NFJPHMKKEBF::LILLELPNAGA` @0x1a2e528 (Match.Initialize
— builds "MatchContainer") when the battle-flow FSM enters the gameplay scene.

**cmd 110 BattleStart is what drives that transition — for CS too, not just BR.**
Handler `KPDMJKOEHEE::FAAJCNCINNK` @0x19458f0:
```
EventLogger::LogLeaveWaitingIslandBattleStarted(1)
GameFacade.GameServerMatchID     = msg.FBDKEAHAEGO
GameFacade.GameServerServiceMatchID = msg.DGCJANAMDBJ
GameFacade::LoadMPBattleGame(CREATE_NEW_CONN)   // leave waiting island -> load battle scene
```
Mode 15's config uses `PVP_WaitingIsland`/`SCENE_BATTLE_WAITING`, so after join the CS
player sits on the WAITING ISLAND ("mode animation, never progresses"). cmd 110 makes it
`LoadMPBattleGame` → load the real battle scene → `LILLELPNAGA` → state=Running → GRI/PRI
accepted. Message `JAKKPBMHLAI::UnSerialize` @0x371e560 = `string key | u64 roomID
(DGCJANAMDBJ) | u64 battleID (FBDKEAHAEGO)` (matches `message.BattleStart`).

**CREATE_NEW_CONN caveat (VERIFIED live):** `LoadMPBattleGame` opens a NEW UDP
connection — the client reconnects (fresh HELLO + cmd 440 join) as the battle scene.
So BattleStart must be sent only on the FIRST (waiting-island) connection; sending it
again on the battle connection loops it back to reload forever. The server tracks
"already sent BattleStart" per account (survives the reconnect via the JWT account id).
Flow: `conn#1 join (waiting) -> cmd 110 -> conn#2 join (battle, state=Running) -> GRI`.
