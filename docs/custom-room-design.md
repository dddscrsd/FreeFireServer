# Custom Room System — Coupled TCP-Lobby + Go-Match Design

**Feature:** Host-configurable private Clash-Squad (CS) matches. Couples the Node TCP lobby tier (`src/tcp`) and the Go UDP match-server (`match-server/**`).
**Status of the wire:** Our protobuf is already complete and round-trips (33/33 room messages, 13/13 enums, verified empirically against `src/protocol/protos.js`). Nothing is wired on either tier. This document is the build spec.

---

## 1. How custom rooms actually work (client-driven lifecycle)

Custom rooms ride **`EProtocol::Proto::ROOM = 14` (0xE)** — confirmed three ways: client RE (`RequestDropMatch` disasm `0x319afd4` loads `MOV W1,#0xE`), our `protocol/protos/tcp.proto:1086` (`Proto_ROOM=14`), and the reference server (`tcpp.py:792 CustomRoom=14` → `handle_custom_room`). All client ops go through one send site:

```
COW::ServiceConnectionManager::SendMessageToLobby(EProtocol::Proto=14, uint32 subCmd, protoMsg, byte)  @0x2985698
```

The room manager is `COW::UIModelCustomRoom` (`get_CurrentRoomInfo @0x3196c3c`, `get_MyRoomRole @0x3195034`), holding `m_CurrentRoomInfo` (a `tcp.RoomInfo`) and `MyRoomRole` (`ECustomRoomRole`: Player=0/Spectator=1/Owner=2).

**All room messages are CLEAR-NAMED `tcp::` types** (`tcp.RoomCreateReq`, `tcp.RoomInfo`, …), *not* the 11-char obfuscated `message::` namespace used by the UDP match protocol. Our extracted `tcp.proto` matches the client's subcmd enum **bit-for-bit**.

### Lifecycle (confirmed, high confidence)

| Step | Client method (`UIModelCustomRoom::`) | C2S op |
|---|---|---|
| **Create** | `RequestCreateRoom @0x319799c` | ROOM/**2** `RoomCreateReq` (name, code, map_id, game_mode, max_member_num, max_spectator_num, room_type, group_id, **room_setting, room_setting2, cs_advanced_setting**) |
| **List** | `RequestRoomList @0x319769c` | ROOM/**1** `RoomListReq` |
| **Join** | `RequestJoinRoom @0x31990b8` | ROOM/**3** `RoomJoinReq` (room_id, code, group_name, inviter_account_id). *If the player is in a party, it instead sends `GROUP`-proto/**19** `GroupJoinRoomReq`* |
| **Spectate** | `RequestSpectateRoom @0x3199ca0` | ROOM/**4** `RoomSpectateReq` |
| **Configure** | `RequestChangeRoom @0x3198ce4` | ROOM/**13** `RoomChangeReq` (name, code, map_id, game_mode, max_member_num, **room_setting, room_setting2, cs_advanced_setting**) |
| **Ready toggle** | `RequestSetReady @0x319aff4` | ROOM/**24** `RoomSetReadyReq` (ready, set_group=false) |
| **Switch seat / role** | `RequestSwitchSeat @0x319b174` | ROOM/**20** `RoomSwitchSeatReq` (account_id, to_role `ERoom.PlayerRole`, to_room_pos, to_group_pos) |
| **Kick** | `RequestKickPlayer @0x319bbec` | ROOM/**9** `RoomKickReq` (room_id, kick_account_id) |
| **Start** | `RequestStartGame @0x319abfc` | ROOM/**11** `RoomStartReq` (room_id) |
| **Leave / drop** | `RequestLeaveRoom @0x319a4f4` / `RequestDropMatch @0x319aeec` | ROOM/**6** `RoomLeaveReq` / ROOM/**15** (null body) |
| Poll info / invite / maps / group move | `RequestRoomInfo`/`RequestInvite`/`RequestUpdateMaps`/`RequestSwitchGroup` | ROOM/**12**, **22**, **33**, **36** |

### Server→client dispatch (confirmed)

`COW::LobbyServiceConnectionHandler::OnMsgCustomRoom(proto::MessageNotify) @0x1b568b0` switches on `(MessageNotify.subCmd − 1)` via jump table `jpt_1B573C8 @0x4f88c18` (40 cases; `subCmd @obj+0x20`, payload bytes `@obj+0x28`). Each case runs `TCPClientMessageUtil::UnSerialize<T>`:

| S2C subcmd | Proto | Client action |
|---|---|---|
| 1 ROOMLIST | `RoomListRes` | `UpdateRoomList` |
| 2 CREATE-resp | `RoomInfo` | HideWaiting(CreateRoom) |
| 5 JOIN_NTF | `RoomJoinNtf` | roster add |
| 7 LEAVE_NTF | `RoomLeaveNtf` | roster remove |
| 8 DISMISS_NTF | `RoomDismissNtf` | `OnCustomRoomDismissed` |
| 10 KICK_NTF | `RoomKickNtf` | — |
| 12 ROOMINFO | `RoomInfo` | `UpdateCurrentRoomInfo` (full refresh) |
| 14 CHANGE_NTF | `RoomInfo` | settings re-render |
| **16 MATCHMAKINGSUSS_NTF** | (match-found payload) | **the start handoff** |
| 17 ROOMSTATE_NTF | `RoomStateNtf{room_id,state}` | `UpdateRoomState` |
| 21/23/25/40 | SWITCHSEAT/INVITE/SETREADY/COUNTDOWN _NTF | broadcasts |

**Membership / host model:** `tcp.RoomInfo{ id(1), name, owner(uint64 host account-id, 3), owner_online(30), state, code(join-code, 10), groups(List<RoomGroupInfo>=teams), spectators(List<RoomPlayerInfo>), room_type, map_id, game_mode, max_member_num, level_visual_style(17), room_setting(18), room_setting2(19), is_cs_advanced(21), cs_advanced_setting(22) }`. Team = `RoomGroupInfo{id,name,abbr_name,members}`; member = `RoomPlayerInfo{account_id,nickname,ready,role,group_id,rank,cs_rank,…}`.

**Low-confidence / unverified:**
- **No dedicated transfer-host op** exists in `UIModelCustomRoom` (full method enumeration). Host = `RoomInfo.owner`; role changes flow through `RequestSwitchSeat` (subcmd 20). *Confidence: medium.* Safe assumption: model ownership purely as `RoomInfo.owner`, rebroadcast via ROOMINFO(12)/SWITCHSEAT_NTF(21).
- **Room subcmd-16 payload type** is inferred to reuse the matchmaking suss shape (reference `tcpp.py` sent `{server_addr, secret, match_id, prepare_token, map_id, game_mode, match_mode}` under CMD 14). Safe assumption: encode `MatchmakingSussNtf` for room-16 too; verify in-client.
- **Enum FQN conflict between reports:** one RE names the subcmd enum `tcp.ECustomRoom.Proto`, the empirical protos-harness resolved it as **`tcp.ERoom.Proto`** (and warns `ERoom.Proto` "only exists as `.tcp.ERoom.Proto`"). **Use `tcp.ERoom.Proto`**; confirm with `lookupEnum` before wiring `protocol.js`.
- The reference `tcpp.py` room tier is **JSON, not protobuf**, with mismatched field names (`room_id/creator_id/players` vs proto `id/owner/groups`). It is a **flow/logic reference only**. Whether retail FF accepts JSON on proto-14 is unverified, but the full proto schema strongly implies protobuf — **use protobuf**.

---

## 2. The settings schema

All CS custom-room settings ride **inside `tcp.RoomInfo`** (and the mirror fields on `RoomCreateReq`/`RoomChangeReq`), across two `uint32` bitfields, one bool, and one opaque bytes blob:

| RoomInfo field | # | Type | Role |
|---|---|---|---|
| `level_visual_style` | 17 | uint32 | Weather |
| `room_setting` | 18 | uint32 | Bitfield, `COW.ECustomRoomSetting` |
| `room_setting2` | 19 | uint32 | Bitfield, `COW.ECustomRoomSetting2` |
| `is_cs_advanced` | 21 | bool | Advanced-CS economy enabled |
| `cs_advanced_setting` | 22 | bytes | Raw LE `BinaryWriter` blob (NOT protobuf) |

> Field-number caveat: `RoomCreateReq` carries the same settings at **different tag numbers** than `RoomInfo` (create: `room_setting=14`, `room_setting2=18`, `cs_advanced_setting=22`). Encode against the exact message. Definitive enum text already lives in-repo at `protocol/protos.bak/COW.proto:731-775` (dropped from active `protos/` — the bitmask enums must be restored or referenced there).

The authoritative client reader that maps every bit is `COW::UIModelCustomRoom::GenerateCustomRoomGameSettingDict @0x31a57b0`; the host writer is `UICreateRoomController::SetRoomSettingValue @0x294683c` (spreads a value across a bit range) + `GCommon::BitArray::AddFlag/RemoveFlag` for single bits.

### `room_setting` (field 18) — `COW.ECustomRoomSetting`

| Setting | Bit / mask | In-match effect |
|---|---|---|
| HideKillInfo | `0x1` (bit0) | Hide kill feed |
| **UnlimitedAmmo** | `0x2` | No reload/ammo consumption (client-consumed flag) |
| NoFallingDamage | `0x4` | — |
| NoLoadout | `0x8` | No starting loadout |
| NoAirdrop | `0x10` | — |
| **NoSkill** | `0x20` | Character skills off |
| NoVehicle | `0x40` | — |
| PlayerHP | bits 8-10 (`0x100`–`0x400`) | HP **index** into config table |
| PlayerEP | bits 11-13 (`0x800`–`0x2000`) | EP index |
| PlayerSpeed | bits 14-16 (`0x4000`–`0x10000`) | Move-speed index |
| DropList | bits 17-20 | BR-oriented |
| PlayerJumpHeight | bits 21-23 (`0x200000`–`0x800000`) | Jump index |
| AccTotalStats | bit 24 | BR-oriented |
| **RoundNum** | bits 25-26 (`0x2000000`–`0x4000000`) | **Round-count index** |
| **InitCoin** | bits 27-28 (`0x8000000`–`0x10000000`) | **Starting-money index** |
| NoPowerGun | `0x20000000` | Gun-attribute lock |
| HideEnemyCloth | `0x40000000` | — |

### `room_setting2` (field 19) — `COW.ECustomRoomSetting2`

`NoUAV=1, NoBomb=2, Replay=4, NoZeppelin=8, NoHud=16, FriendDmg=32 (friendly fire), FightClubRoundNum=bits6-7, ReviveSwitch=bits8-10 (revive/respawn rule), InGameChat=2048, ShopFlow=4096, UseRandomMap=8192, NoAuxAim=16384 (aim-assist off).`

### `cs_advanced_setting` (field 22) — advanced-CS economy blob

Raw little-endian `BinaryWriter` layout (reconstructed from `GenerateADCSSettingBytes @0x31983c0`, high confidence but **verify against a captured blob**):

```
[u32 shopItemCount]  shopItemCount × { u32 itemId, u32 price }
[u32 ecoCount]       ecoCount      × { u32 itemId, u32 value }
[u32 roundCount]     roundCount    × u16 perRoundCoin
```

- **Banned/disabled weapons are simply omitted** from the shop-item pairs (`m_AdCSShopSettingCheckDic[key]==false`).
- Defaults come from `GameVarDef.CustomRoomCSInitCoin` (`;`-separated per-round table), `CustomRoomCSMaxRound` (pads with last value), `CustomRoomCSShopCostInterval` (price→eco divisor).
- When `is_cs_advanced` is set, **round count and starting money come from the blob** (`AdCSEcoRound`, `GetCSRoundValueByIndex(0)`) instead of the `room_setting` RoundNum/InitCoin bits.

### The three settings the user named — mapping to what the match must do

| User-named setting | Simple mode (`room_setting`) | Advanced mode (`cs_advanced_setting`) | Match-server target |
|---|---|---|---|
| **Round count** | RoundNum bits 25-26 (index → RoundNumConfig table) | `roundCount` / `AdCSEcoRound` (raw) | `maxRound`/`roundsToWin` (`cssync.go:13-14`) → GRI `CSGRIInit` (`match.go:431`), end gate (`match.go:762`) |
| **Allowed shop guns** | coarse only: NoLoadout(`0x8`), NoPowerGun(`0x20000000`) | shop-item pairs `{itemId,price}` — **omitted = banned**, price overridable | `csShopItems`/`csShopTitle` (`shop.go:7,17`) → `sendCSShop` (cmd 407, `cssync.go:230`) + `shopItemByID` validation (`shop.go:45`) |
| **Per-round money bonus** | InitCoin bits (starting only, **no per-round table**) | `perRoundCoin[]` u16 table | starting `startingCoins` (`purchase.go:15`) + the three bare `500` literals at `match.go:752-755` |

**Guidance:** the per-gun allow-list and the per-round money table **only exist in advanced mode**. To honor the user's exact three settings deterministically, drive custom rooms with `is_cs_advanced=true` — the blob carries raw values and sidesteps the index→value config-table dependency that simple mode requires (see Risks).

**Client-side consumption (deferred):** at match time the client re-reads `room_setting`/`room_setting2` from an in-match settings message `message.JCPLHHBPKPC` (fields `GPKHLEAADHL=room_setting`, `OJIHIKIAFAI=room_setting2`, read by `KPDMJKOEHEE.LLEOKLJJPJD @0x19803c4`) and sets `UIModelMatch.IsCustomRoomSettingFriendDmg/UnlimitedAmmo/NoHud`. These flags need a client message we cannot yet encode (wire tags unconfirmed) — **not required for the three named settings**, which are fully server-enforced.

---

## 3. Wire map

Protocol = **14 (ROOM)** for everything below. C2S `ProtoReq.cmd` = subcmd; S2C `MessageNotify.subCmd` = subcmd.

| Op | Dir | proto/cmd | Proto message | Content |
|---|---|---|---|---|
| List rooms | C2S | 14 / 1 | `tcp.RoomListReq` | room_type, game_modes[], room_tab_type[] |
| List result | S2C | 14 / 1 | `tcp.RoomListRes` | room_list[] (`tcp.RoomBasicInfo`) |
| Create | C2S | 14 / 2 | `tcp.RoomCreateReq` | name, code, map_id, game_mode, max_member_num, room_type, **room_setting, room_setting2, cs_advanced_setting** |
| Create result | S2C | 14 / 2 | `tcp.RoomInfo` | full room state |
| Join | C2S | 14 / 3 | `tcp.RoomJoinReq` | room_id, code, group_name, inviter_account_id |
| Join broadcast | S2C | 14 / 5 | `tcp.RoomJoinNtf` | new member |
| Spectate | C2S | 14 / 4 | `tcp.RoomSpectateReq` | room_id, code |
| Leave | C2S | 14 / 6 | `tcp.RoomLeaveReq` | room_id |
| Leave broadcast | S2C | 14 / 7 | `tcp.RoomLeaveNtf` | account_id |
| Dismiss | S2C | 14 / 8 | `tcp.RoomDismissNtf` | reason (`ERoom.DismissReason`) |
| Kick | C2S | 14 / 9 | `tcp.RoomKickReq` | room_id, kick_account_id |
| Kick broadcast | S2C | 14 / 10 | `tcp.RoomKickNtf` | account_id |
| **Start** | C2S | 14 / 11 | `tcp.RoomStartReq` | room_id |
| Room info (poll) | C2S | 14 / 12 | `tcp.RoomInfoReq` | room_id, room_type |
| **Room refresh** | S2C | 14 / 12 | `tcp.RoomInfo` | full state (broadcast to all members after any mutation) |
| Change settings | C2S | 14 / 13 | `tcp.RoomChangeReq` | name, map_id, game_mode, max_member_num, **room_setting, room_setting2, cs_advanced_setting** |
| Change broadcast | S2C | 14 / 14 | `tcp.RoomInfo` | updated state |
| **Match handoff** | S2C | 14 / 16 | `MatchmakingSussNtf`* | server_addr, secret, match_id, prepare_token, map_id, game_mode, match_mode |
| Room state | S2C | 14 / 17 | `tcp.RoomStateNtf` | room_id, state (IDLE/INGAME) |
| Switch seat | C2S | 14 / 20 | `tcp.RoomSwitchSeatReq` | account_id, to_role, to_room_pos, to_group_pos |
| Seat broadcast | S2C | 14 / 21 | (RoomInfo/seat ntf) | — |
| Set ready | C2S | 14 / 24 | `tcp.RoomSetReadyReq` | ready, set_group |
| Ready broadcast | S2C | 14 / 25 | `tcp.RoomSetReadyNtf` | account_id, ready |
| Countdown | S2C | 14 / 40 | `tcp.RoomCountDownNtf` | seconds |

\* Payload type for room-16 unconfirmed; reuse `MatchmakingSussNtf` (reference precedent).

**Encoding gotcha:** `RoomBasicInfo` and `ERoom` exist in **both** `tcp.proto` and `proto.proto` with different shapes; `lookup()` returns the `proto.proto` variant. **Always encode/decode with fully-qualified `tcp.*` names** (`lookup('tcp.RoomInfo')`, `lookupEnum('tcp.ERoom.Proto')`).

---

## 4. Implementation plan (file-by-file)

### (A) TCP lobby room service — Node side

**`src/tcp/protocol.js`** (currently exposes `EProtocol/EFriend/EPresence/EStats/EMatchmaking` via `enumValues()`, lines 34-40). Add:
```js
ECustomRoom:    enumValues('tcp.ERoom.Proto'),     // subcmd names (verify FQN with lookupEnum first)
ECustomRoomErr: enumValues('tcp.ERoom.ErrCode'),
```
so handlers reference named subcmds/error codes (mirrors `EMatchmaking`).

**`src/tcp/handlers/Room*.js`** — new modules, auto-registered by `router.js:28-45` (keyed `(protocol,subcmd)`, files starting `_` skipped). Each exports `{ protocol: 14, subcmd, reqType:'tcp.RoomXReq', resType:'tcp.RoomInfo'|…, resCmd, handler(reqObj, ctx) }`. `resCmd` routes replies under the *_NTF subcmd (e.g. START→16, SETREADY(24)→SETREADY_NTF(25)). MVP handler set: `RoomCreate`(2), `RoomList`(1), `RoomJoin`(3), `RoomLeave`(6), `RoomInfo`(12), `RoomChange`(13), `RoomSetReady`(24), `RoomKick`(9), `RoomStart`(11).

**Room-state store — Redis-backed (recommended).** The room roster must survive the gateway pool (any gateway node can receive any member's request), exactly like the matchmaker's shared queue. Mirror `src/tcp/matchmaker.js`'s `getBus()` pattern:
- `room:seq` → `bus.incr()` for room ids (mirrors matchmaker `SEQ_KEY`).
- `room:{id}` → JSON of the full `tcp.RoomInfo` (settings stored **verbatim/opaque** — the lobby never decodes the bitfields; it stores and rebroadcasts them; decoding happens match-side).
- `room:index` (hash) → room id → summary for `RoomListReq`.
- `room:member:{accountId}` → room id, for reverse lookup and disconnect cleanup (mirror the reference's `handle_room_leave` on disconnect, `tcpp.py:5121`).

**How settings are held:** the host's `RoomChange(13)` (owner-guarded: `req.owner_account == RoomInfo.owner`) writes `room_setting`/`room_setting2`/`is_cs_advanced`/`cs_advanced_setting` straight onto the stored `RoomInfo`. No decode. After any mutation, broadcast the full `RoomInfo` to every member.

**Broadcasts** use the same presence-routed push `formMatch` already uses (`matchmaker.js:217`):
```js
bus.publishPS('gw.push', 'GatewayPush', { target_account_id, protocol: 14, cmd: <subcmd_ntf>, content });
```
Fan out ROOMINFO(12) to all members on join/leave/change/ready, plus the specific *_NTF (5/7/10/25).

**Start handoff (`RoomStart`, subcmd 11):** reuse the matchmaker machinery in `matchmaker.js:185 formMatch` — `bus.incr(SEQ_KEY)` for the match id, `fleet.pickServer()||staticAddr()`, `buildPrepareToken(account, matchId)` per member. Then push **ROOM/16** (not MATCHMAKING/16) carrying the suss payload, flip room state → INGAME via ROOMSTATE_NTF(17). Presence: `PresenceList.js` already models INROOM state — update it on join/leave.

### (B) Settings propagation TCP → match — **Redis-by-match-id (recommended)**

Three candidate channels evaluated:

1. **Token JWT (`token.Claims`, per-player).** ✗ Each player carries their **own** `prepare_token`; match-wide settings would have to be identical across every token (the "admitFirst is authoritative" hack is fragile). The `cs_advanced_setting` blob would bloat every token, and the join token **already splits across cmd 439/440** (per prior RE) — adding a blob worsens fragmentation.
2. **A new UDP command.** ✗ No such path exists; adds ordering/auth surface for no benefit.
3. **Redis key keyed by match id (RECOMMENDED).** ✓ Settings are **match-scoped (one per match)**, and:
   - the matchmaker/room-start already **owns the match id** (`bus.incr(SEQ_KEY)`) and a bus (`getBus()`);
   - the **match-server already connects to the same bus** (`config.go:36 redisURL`, `nodeID`, and it already publishes results via the bus in `results.go`);
   - it keeps the per-player token small and avoids the multi-token-consistency problem.

**Design:** at room-start, write `match:{id}:settings` (JSON: `roundCount, roundsToWin, startCoins, perRoundCoins[], shopItems[{itemId,price,filter,limitation}], shopTitle, maxHP, maxEP, flags{friendlyFire,unlimitedAmmo,noSkill,noPowerGun,noAuxAim,reviveRule,noHud}`). The lobby decodes `room_setting`/`room_setting2`/`cs_advanced_setting` **here, once**, into this normalized JSON (the Go side stays free of bit-twiddling). The `prepare_token` keeps carrying only identity+cosmetics+`mid`. The match-server does one `GET match:{mid}:settings` at match creation. If the match-server has no bus (`redisURL` empty / standalone), it falls back to the current consts — zero behavior change for non-room matches.

### (C) match-server changes — Go side

Every knob today is a package-level `const` or a bare literal, and `config.go`'s `cfg` is **process-global** (`config.go:44 var cfg`) — one per instance, shared by every match, so it **cannot** hold per-room settings. Introduce a **per-`Match`** settings struct.

**New type + field** (default from the current consts):
```go
type matchSettings struct {
    maxRound, roundsToWin uint8
    startCoins            uint32
    perRoundCoins         []uint32          // per-round money bonus table
    shopItems             []message.ShopItem // allowed guns + prices
    shopTitle             string
    maxHP, maxEP          uint16
    friendlyFire, unlimitedAmmo, noSkill, noPowerGun, noAuxAim, noHud bool
    reviveRule            uint8
}
```
Add `settings matchSettings` on `*Match` (`match.go`), populated in `admitFirst`/`addPlayer` (`matchhandle.go`, gated by `canAdmit` at `matchhandle.go:39`).

**Parse at join.** At the first join (`admitFirst`), read `match:{MatchID}:settings` from the bus using `MatchID` from the token (`resolvePlayer`, `player.go:83` → `PlayerInfo.MatchID`, `playerjoin.go`). Decode JSON into `m.settings`; on miss/no-bus, keep the const defaults.

**Consume — extract the literals first, then override:**

- **Round loop:** replace `maxRound`/`roundsToWin` (`cssync.go:13-14`) reads with `m.settings.*` at the end gate (`match.go:762 if m.teamScore[0] >= roundsToWin || … || m.round >= maxRound`) and the GRI stream (`match.go:431 CSGRIInit(maxRound, m.round-1)`, `replication.go` `CSFieldMaxRound=1`/`CSFieldCurrentRound=2`) — the client round bar **auto re-renders**, no client wiring. Match-point flag (`match.go:435`) uses the derived `roundsToWin`.
- **Per-round money:** the three **unnamed** `500` literals at `match.go:752-755` (`aw := 500*roundKills; aw += 500*(round+1); if win aw += 500`) must first be extracted into fields, then sourced from `m.settings.perRoundCoins[m.round]` (+ per-kill / win terms). Starting money `startingCoins` (`purchase.go:15`) → `m.settings.startCoins`, applied at seed (`match.go:356`) and `giveLoadout` reset (`cssync.go:104`).
- **Allowed shop guns:** `csShopItems`/`csShopTitle` (`shop.go:7,17`) → `m.settings.shopItems`/`shopTitle`, sent by `sendCSShop` (cmd 407, `cssync.go:230`) and validated by `shopItemByID` (`shop.go:45`) — a gun omitted from the host's allow-list is simply absent from both the catalogue and buy validation. (There is already a `shop.go:15` TODO for per-round re-pricing.)
- **HP / EP:** `maxHP=200` (`player.go:25`, read by `initHP` `combat.go:25`, `entityHP` `combat.go:41`, PRI payload `cssync.go:42`) → `m.settings.maxHP`; `maxEP`/`epPerMushroom`/`mushroomsPerRound` (`ep.go:13-18`) → settings.
- **Flags (new handlers):** friendly fire keys off `m.settings.friendlyFire` in `creditDeath`/damage application (`deathmatch.go:142` currently credits only cross-team kills — friendly-fire-on must allow same-team damage); `noSkill`, `unlimitedAmmo`, `noPowerGun`, `reviveRule` are new gates. **These have no existing hooks.**

**Do NOT touch** the durable settlement economy (`results.go:16` `settleCoinPerKill=50/settleWinCoins=200/settleXPPerKill/…`) — that's the lobby-reward path in `publishResult`, independent of in-match buying.

---

## 5. Phasing (build order, each independently testable)

**Phase 0 — Proto/enum exposure + store scaffold.** Add `ECustomRoom` to `protocol.js`; stand up the Redis room store. *Test:* proto encode/decode harness (already proven: 33/33 round-trip) + Redis CRUD unit test. No client.

**Phase 1 — Room lifecycle + membership.** Handlers for create/list/join/leave/kick/info/setready with ROOMINFO(12) fan-out. *Test in live client:* host creates a room, second guest joins by code, ready toggles broadcast, host sees the roster update. **No match yet.**

**Phase 2 — Start handoff (default settings).** `RoomStart(11)` → reuse `formMatch` → ROOM/16 suss → clients enter a **normal CS match with current const settings**. *Test:* room start launches a working match (validates the whole handoff independent of settings).

**Phase 3 — Settings capture + propagation.** Store `room_setting`/`room_setting2`/`cs_advanced_setting` on `RoomInfo` via `RoomChange(13)`; at start, decode into `match:{id}:settings`. *Test:* match-server logs the decoded settings at join (no application yet).

**Phase 4 — Match-side application of the three named settings**, in order of testability:
1. **Round count** → GRI round bar visibly reflects the host's pick.
2. **Per-round money** → buy-phase coins match the host's table.
3. **Allowed shop guns** → cmd 407 catalogue shows only permitted guns at host prices.
Then HP/EP and the server-enforced flags (friendly fire, noSkill, noPowerGun).

**Phase 5 (deferred) — client-consumed flags** (NoHud, UnlimitedAmmo prediction, FriendDmg display) via the in-match settings message `message.JCPLHHBPKPC` — blocked on RE of its cmd number and field tags.

---

## 6. Open questions & risks

**Needs an in-game check / deeper RE:**
- **`JCPLHHBPKPC` wire format** (in-match settings message, cmd + field tags) is unknown — it is *not* in our extracted `protos/`; struct offsets only imply `list, u32, u32`. Required for client-rendered flags (Phase 5), not for the three named settings.
- **`match_mode` for rooms.** Reference `tcpp.py` sends `EGAMETYPE.Room = 3`; our matchmaker **forces `match_mode:6`** (`matchmaker.js:211`) so the client renders the CS waiting/ranking HUD — and prior RE notes the countdown HUD **only exists when `GameServerMatchMode==6`**. A room match may need 3, 6, or a room-specific mode; the wrong choice either loses the HUD or writes ranked leaderboards. **Test both.**
- **`cs_advanced_setting` byte layout** is reconstructed from `GenerateADCSSettingBytes` (high confidence, not captured). Unit-test the decoder against a real host-emitted blob; confirm the `perRoundCoin` u16 width and the `CustomRoomCSShopCostInterval` price↔eco divisor.
- **Simple-mode index→value tables** (HP/EP/speed/jump/RoundNum/InitCoin/Revive) live in **server config (`RoomCreateRuleRes` CSV), not the client binary** — the bitfield stores only a small **index**. If we support simple mode, the lobby must ship a `RoomCreateRule` config whose value tables match what the client displays, or numeric picks desync. **Mitigation: drive custom rooms in `is_cs_advanced` mode** (raw values in the blob) for the named settings.
- **Enum FQN** (`tcp.ERoom.Proto` vs `tcp.ECustomRoom.Proto`) — reports conflict; resolve empirically with `lookupEnum` before Phase 0.
- **No transfer-host op** (medium confidence) — confirm ownership only ever moves via `RoomSwitchSeat`/server rebroadcast.

**Where our current code fights this:**
- **`config.go cfg` is process-global** — the entire existing override pattern (`unlimitedMoneyTest`, `infiniteGloo`, `matchBot`, `buyPhase`) is one-per-instance and **cannot** carry per-room settings. Per-`Match` fields are mandatory.
- **The match-server is built for solo-vs-passive-bot, not true PvP.** `endFight` decides the round winner by `localWon := m.teamAlive(localTeamID) > 0` with fixed `localTeamID=1/enemyTeamID=2` (`round.go:17-18`), and only the local team scores. A real 4v4 human custom room breaks this survivorship model — **this is the single biggest architectural risk** and likely needs `endFight`/`teamScore` reworked for two active human teams before rooms are truly playable (Phase 2+ will expose it).
- **`maxPlayers=8` is fixed** (`manager.go:9`, gates `canAdmit` at `matchhandle.go:39`) — a room's `max_member_num` above 8 total won't admit everyone.
- **The coin literals (`match.go:752-755`) are unnamed** and `giveLoadout` ids are inline function-local consts (`cssync.go:97`: `uspData=3`, `pistolAmmo=202`, `medkitData=102`, `glooDataID=1201`) — these must be **extracted into fields/vars before** any override can hook them.
- **Reference room tier is JSON** — logic reference only; build on protobuf, not `tcpp.py`'s wire.

---

# Appendix — raw recon findings

## TCP lobby custom-room (private match) lifecycle — client RE (FF 1.70.1 il2cpp ARM64)

The custom-room lifecycle rides EProtocol::Proto::ROOM = 14 (0xE). The client-side room manager is COW::UIModelCustomRoom; every C2S op is a method RequestX that calls COW::ServiceConnectionManager::SendMessageToLobby(EProtocol::Proto ROOM=14, uint32 subCmd, protoMsg, byte) @0x2985698. The server->client handler is COW::LobbyServiceConnectionHandler::OnMsgCustomRoom(proto::MessageNotify) @0x1b568b0, a switch on (subCmd-1) via jump table jpt_1B573C8 @0x4f88c18. All proto messages are CLEAR-NAMED tcp:: types (NOT the 11-char obfuscated message:: namespace) — tcp.RoomCreateReq/RoomJoinReq/RoomInfo/RoomListRes/RoomStateNtf/etc. Confirmed subcmds — create=2, join=3, spectate=4, leave=6, kick=9, start=11, roominfo=12, change=13, dropmatch=15, switchseat=20, invite=22, setready=24, changemaps=33, ownerswitchgroup=36; S2C notifies — join_ntf=5, leave_ntf=7, dismiss_ntf=8, kick_ntf=10, roominfo=12, change_ntf=14, roomstate_ntf=17. These match OUR protocol/protos/tcp.proto CustomRoom subcmd enum (Proto_ROOM=14 line 1086; subcmd enum ROOMLIST=1..COUNTDOWN_NTF=40 lines ~1126-1165) EXACTLY, so our proto is already correct; what is missing is (a) exposing that subcmd enum in src/tcp/protocol.js and (b) adding (14,subcmd) handler modules under src/tcp/handlers/. Room membership/host/code model: tcp.RoomInfo{ id(1), owner(uint64 account-id=host,3), code(string join-code,10), state, groups[RoomGroupInfo]=teams, spectators[], owner_online(30), room_type }. There is NO dedicated transfer-host op; host = RoomInfo.owner and role/seat changes flow through RequestSwitchSeat (subcmd 20, ERoom::PlayerRole).

- **Custom-room protocol = EProtocol::Proto::ROOM = 14 (0xE); all room ops send via ServiceConnectionManager::SendMessageToLobby(EProtocol::Proto, uint32 subCmd, Object msg, byte).**
  - evidence: RequestDropMatch disasm 0x319afd4: MOV W1,#0xE ; MOV W2,#0xF then B SendMessageToLobby (0x2985698). Every RequestX passes EProtocol_Proto__Enum_ROOM as arg1.
  - symbols: _ZN3COW24ServiceConnectionManager18SendMessageToLobbyEN3tcp9EProtocol5ProtoEjN6System6ObjectEh @0x2985698; EProtocol_Proto__Enum_ROOM
  - protos: tcp.EProtocol.Proto ROOM=14
  - cmds: ROOM=14
  - files: protocol/protos/tcp.proto:1086; src/tcp/protocol.js
  - confidence: high
- **Room manager class is COW::UIModelCustomRoom; it holds m_CurrentRoomInfo (tcp.RoomInfo), MyRoomRole (ECustomRoomRole: Player=0/Spectator=1/Owner=2), m_RoomListDict, m_ModelGroup. get_CurrentRoomInfo @0x3196c3c, get_MyRoomRole @0x3195034.**
  - evidence: func_query COW::UIModelCustomRoom methods; RequestKickPlayer reads this->m_CurrentRoomInfo->get_id (0x4b51380).
  - symbols: _ZN3COW17UIModelCustomRoom19get_CurrentRoomInfoEv @0x3196c3c; _ZN3COW17UIModelCustomRoom14get_MyRoomRoleEv @0x3195034; COW::ECustomRoomRole
  - confidence: high
- **CREATE ROOM = ROOM/subcmd 2, message tcp.RoomCreateReq.**
  - evidence: RequestCreateRoom builds tcp::RoomCreateReq (set_map_id/game_mode/room_name/code/max_member_num/max_spectator_num/room_type/group_id/room_setting...) then SendMessageToLobby(ROOM, 2u, req). Also shows ShowWaiting(CreateRoom).
  - symbols: _ZN3COW17UIModelCustomRoom17RequestCreateRoomEN3COW10RoomParamsE @0x319799c; _ZN8TypeInfo3tcp13RoomCreateReqE
  - protos: tcp.RoomCreateReq (name/code/max_member_num/max_spectator_num/room_type/group_id/room_setting)
  - cmds: ROOM=14/subcmd=2 CREATE
  - files: protocol/protos/tcp.proto:2289
  - confidence: high
- **JOIN ROOM (by id + room-code) = ROOM/subcmd 3, message tcp.RoomJoinReq; if the player is in a group it instead sends GROUP proto/subcmd 19 with tcp.GroupJoinRoomReq.**
  - evidence: RequestJoinRoom: solo path v39=ROOM,v40=3,msg=RoomJoinReq(set_room_id,set_code,set_group_name,set_inviter_account_id); group path v39=GROUP,v40=19,msg=GroupJoinRoomReq. Final COW::ServiceConnectionManager::SendMessageToLobby(v38,v39,v40,v41).
  - symbols: _ZN3COW17UIModelCustomRoom15RequestJoinRoomEmN6System6StringES2_S2_bbmj @0x31990b8
  - protos: tcp.RoomJoinReq (room_id, code, group_name, inviter_account_id, available_maps); tcp.GroupJoinRoomReq
  - cmds: ROOM=14/subcmd=3 JOIN; GROUP/subcmd=19 (group path)
  - confidence: high
- **SPECTATE ROOM = ROOM/subcmd 4, message tcp.RoomSpectateReq (solo) or tcp.GroupSpectateRoomReq (group).**
  - evidence: RequestSpectateRoom disasm 0x319a1b8: MOV W1,#0xE; MOV W2,#4; builds tcp::RoomSpectateReq::set_room_id/set_code (solo) or GroupSpectateRoomReq::set_room_id/set_code/set_group_id/set_is_solo/set_room_type (group).
  - symbols: _ZN3COW17UIModelCustomRoom19RequestSpectateRoomEmN6System6StringEbj @0x3199ca0; _ZN8TypeInfo3tcp15RoomSpectateReqE
  - protos: tcp.RoomSpectateReq; tcp.GroupSpectateRoomReq
  - cmds: ROOM=14/subcmd=4 SPECTATE
  - confidence: high
- **LEAVE ROOM = ROOM/subcmd 6, message tcp.RoomLeaveReq (room_id from m_CurrentRoomInfo). Batch-leave uses the same subcmd 6.**
  - evidence: RequestLeaveRoom: SendMessageToLobby(ROOM, 6u, RoomLeaveReq{room_id}) then ClearData. RequestBatchLeaveRoom @0x319a7fc also sends ROOM,6u then ClearBatchData.
  - symbols: _ZN3COW17UIModelCustomRoom16RequestLeaveRoomEv @0x319a4f4; _ZN3COW17UIModelCustomRoom21RequestBatchLeaveRoomEm @0x319a7fc
  - protos: tcp.RoomLeaveReq (room_id)
  - cmds: ROOM=14/subcmd=6 LEAVE
  - confidence: high
- **KICK MEMBER = ROOM/subcmd 9, message tcp.RoomKickReq{room_id, kick_account_id}. Guarded by m_CurrentRoomInfo != null (owner-only in practice).**
  - evidence: RequestKickPlayer: RoomKickReq::set_room_id(RoomInfo::get_id), set_kick_account_id(id), SendMessageToLobby(ROOM, 9u, req).
  - symbols: _ZN3COW17UIModelCustomRoom17RequestKickPlayerEm @0x319bbec; _ZN8TypeInfo3tcp11RoomKickReqE
  - protos: tcp.RoomKickReq (room_id, kick_account_id)
  - cmds: ROOM=14/subcmd=9 KICK
  - confidence: high
- **START MATCH = ROOM/subcmd 11 (0xB), message tcp.RoomStartReq{room_id}.**
  - evidence: RequestStartGame: RoomStartReq::set_room_id(id); SendMessageToLobby(ROOM, 0xBu, req). Debug string #RequestStartGame#.
  - symbols: _ZN3COW17UIModelCustomRoom16RequestStartGameEm @0x319abfc; _ZN8TypeInfo3tcp12RoomStartReqE
  - protos: tcp.RoomStartReq (room_id)
  - cmds: ROOM=14/subcmd=11 START
  - confidence: high
- **CHANGE ROOM SETTINGS = ROOM/subcmd 13 (0xD), message tcp.RoomChangeReq (room_name, code, map_id, game_mode, max_member_num, max_spectator_num, enable_death_spectate, room_setting, room_setting2, cs_advanced_setting).**
  - evidence: RequestChangeRoom: builds RoomChangeReq from RoomParams + current RoomInfo id/type, SendMessageToLobby(ROOM, 0xDu, req).
  - symbols: _ZN3COW17UIModelCustomRoom17RequestChangeRoomEN3COW10RoomParamsE @0x3198ce4; _ZN8TypeInfo3tcp13RoomChangeReqE
  - protos: tcp.RoomChangeReq
  - cmds: ROOM=14/subcmd=13 CHANGE
  - files: protocol/protos/tcp.proto:2260
  - confidence: high
- **TOGGLE READY = ROOM/subcmd 24 (0x18), message tcp.RoomSetReadyReq{ready, set_group=false}.**
  - evidence: RequestSetReady: RoomSetReadyReq::set_ready(ready), set_set_group(0); SendMessageToLobby(ROOM, 0x18u, req).
  - symbols: _ZN3COW17UIModelCustomRoom15RequestSetReadyEb @0x319aff4; _ZN8TypeInfo3tcp15RoomSetReadyReqE
  - protos: tcp.RoomSetReadyReq (ready, set_group)
  - cmds: ROOM=14/subcmd=24 SETREADY
  - confidence: high
- **SWITCH SEAT / change member role (also the mechanism for host/role assignment) = ROOM/subcmd 20 (0x14), message tcp.RoomSwitchSeatReq{room_id, room_type, account_id, to_role(ERoom::PlayerRole), to_room_pos, to_group_pos}.**
  - evidence: RequestSwitchSeat: set_account_id/set_to_role/set_to_room_pos/set_to_group_pos; SendMessageToLobby(ROOM, 0x14u, req). There is no separate transfer-owner request in the class.
  - symbols: _ZN3COW17UIModelCustomRoom17RequestSwitchSeatEmN3tcp5ERoom10PlayerRoleEjj @0x319b174; tcp::ERoom::PlayerRole
  - protos: tcp.RoomSwitchSeatReq
  - cmds: ROOM=14/subcmd=20 SWITCHSEAT
  - confidence: high
- **OWNER SWITCH GROUP (move a whole team slot) = ROOM/subcmd 36 (0x24), message tcp.RoomOwnerSwitchGroupReq{room_id, from_group_id/pos, to_group_id/pos}. INVITE = ROOM/subcmd 22 (0x16) tcp.RoomInviteReq{invitee_id}. UPDATE AVAILABLE MAPS = ROOM/subcmd 33 (0x21) tcp.RoomChangeAvailableMapsReq. ROOM LIST = ROOM/subcmd 1 tcp.RoomListReq. ROOM INFO poll = ROOM/subcmd 12 (0xC) tcp.RoomInfoReq. DROP MATCH = ROOM/subcmd 15 (0xF) null body.**
  - evidence: RequestSwitchGroup SendMessageToLobby(ROOM,0x24u); RequestInvite(ROOM,0x16u); RequestUpdateMaps(ROOM,0x21u); RequestRoomList(ROOM,1u); RequestRoomInfo(ROOM,0xCu); RequestDropMatch(ROOM,0xFu,null).
  - symbols: _ZN3COW17UIModelCustomRoom18RequestSwitchGroupEii @0x319b3f8; _ZN3COW17UIModelCustomRoom13RequestInviteEm @0x319b6fc; _ZN3COW17UIModelCustomRoom17RequestUpdateMapsE... @0x319b850; _ZN3COW17UIModelCustomRoom15RequestRoomListE... @0x319769c; _ZN3COW17UIModelCustomRoom15RequestRoomInfoEmN3tcp5ERoom4TypeE @0x319a36c; _ZN3COW17UIModelCustomRoom16RequestDropMatchEv @0x319aeec
  - protos: tcp.RoomOwnerSwitchGroupReq; tcp.RoomInviteReq; tcp.RoomChangeAvailableMapsReq; tcp.RoomListReq; tcp.RoomInfoReq
  - cmds: ROOM=14/subcmd=36 OWNERSWITCHGROUP; subcmd=22 INVITE; subcmd=33 CHANGEAVAILABLEMAPS; subcmd=1 ROOMLIST; subcmd=12 ROOMINFO; subcmd=15 DROPMATCH
  - confidence: high
- **S2C dispatcher = COW::LobbyServiceConnectionHandler::OnMsgCustomRoom(proto::MessageNotify) @0x1b568b0; switches on (MessageNotify.subCmd - 1) via jump table jpt_1B573C8 @0x4f88c18 (40 cases). Handled subcmds: 1,2,5,7,8,10,12,14,16,17,21,23,25,29,30,32,34,35,37,38,40; all others fall through to default.**
  - evidence: disasm 0x1b573a8: LDR W8,[X21,#0x20]; SUB W8,W8,#1; CMP W8,#0x27; ADRL X9,jpt_1B573C8; LDRSW X8,[X9,X8,LSL#2]; ADD X8,X8,X9; BR X8. Jump-table bytes @0x4f88c18 decoded (target=entry+base): default cases 3,4,6,9,11,13,15,18-20,22,24,26-28,31,33,36,39 — exactly the IDA annotation.
  - symbols: _ZN3COW29LobbyServiceConnectionHandler15OnMsgCustomRoomEN5proto13MessageNotifyE @0x1b568b0; jpt_1B573C8 @0x4f88c18
  - protos: proto.MessageNotify (subCmd at obj+0x20, payload bytes at obj+0x28)
  - cmds: ROOM=14 S2C dispatch
  - confidence: high
- **SERVER->CLIENT room-info / room-update broadcasts and their proto messages: subcmd1 ROOMLIST->tcp.RoomListRes->UpdateRoomList; subcmd2 CREATE-resp->tcp.RoomInfo (HideWaiting CreateRoom); subcmd5 JOIN_NTF->tcp.RoomJoinNtf; subcmd7 LEAVE_NTF->tcp.RoomLeaveNtf; subcmd8 DISMISS_NTF->tcp.RoomDismissNtf->OnCustomRoomDismissed; subcmd10 KICK_NTF->tcp.RoomKickNtf; subcmd12 ROOMINFO->tcp.RoomInfo->UpdateCurrentRoomInfo; subcmd14 CHANGE_NTF->tcp.RoomInfo; subcmd17 ROOMSTATE_NTF->tcp.RoomStateNtf{room_id,state}->UpdateRoomState/UpdateBatchRoomState.**
  - evidence: case bodies show GCommon::TCPClientMessageUtil::UnSerialize<T> MethodInfo per case: case1 UnSerializeIN3tcp11RoomListResE; case2/12/14 UnSerializeIN3tcp8RoomInfoE; case5 RoomJoinNtf; case7 RoomLeaveNtf; case8 RoomDismissNtf; case10 RoomKickNtf. UpdateRoomState (0x319e6a0) / UpdateBatchRoomState (0x31a3714 called at 0x1b57fbc, case17). RoomStateNtf getters get_room_id@0x4b51e64/get_state@0x4b51e74.
  - symbols: _ZN3COW17UIModelCustomRoom14UpdateRoomListEN3tcp11RoomListResE @0x319bd70; _ZN3COW17UIModelCustomRoom21UpdateCurrentRoomInfoEN3tcp8RoomInfoEb @0x319db74; _ZN3COW17UIModelCustomRoom15UpdateRoomStateEN3tcp8RoomInfoEN3tcp12RoomStateNtfEb @0x319e6a0; _ZN3COW17UIModelCustomRoom21OnCustomRoomDismissedEN3tcp5ERoom13DismissReasonE @0x31a0450
  - protos: tcp.RoomListRes; tcp.RoomInfo; tcp.RoomJoinNtf; tcp.RoomLeaveNtf; tcp.RoomDismissNtf; tcp.RoomKickNtf; tcp.RoomStateNtf(room_id,state)
  - cmds: subcmd 1/2/5/7/8/10/12/14/17
  - confidence: high
- **Other handled S2C notifications map 1:1 to our tcp.proto CustomRoom subcmd enum: 16 MATCHMAKINGSUSS_NTF (match found), 21 SWITCHSEAT_NTF, 23 INVITE_NTF, 25 SETREADY_NTF (ready-state broadcast), 29 ANTIADDICTION_NFT, 30 TEAMMATEHEATING_NFT, 32 ROOM_CREATE_RULE (reply to RequestRoomCreateRules), 34 CHANGEAVAILABLEMAPS_NTF, 35 EMULATORCHECK_NTF, 37 OWNERSWITCHGROUP_NTF, 38 ROLECHECK_NTF, 40 COUNTDOWN_NTF.**
  - evidence: Handled non-default cases (from jump table) = 16,21,23,25,29,30,32,34,35,37,38,40; these are exactly the *_NTF entries in tcp.proto CustomRoom subcmd enum lines 1141-1165. RequestRoomCreateRules @0x319ba40 pairs with 32.
  - protos: tcp CustomRoom subcmd enum ROOMLIST=1..COUNTDOWN_NTF=40
  - cmds: subcmd 16/21/23/25/29/30/32/34/35/37/38/40
  - files: protocol/protos/tcp.proto:1141
  - confidence: high
- **Room membership / host / id / code data model = tcp.RoomInfo: id(uint64 room-id), name(string), code(string join-code), owner(uint64 = host account-id) + owner_online(bool), state(uint32 RoomState), groups(List<RoomGroupInfo> = teams), spectators(List<RoomPlayerInfo>), room_type(ERoom::Type), map_id/game_mode/group_mode/max_member_num/room_setting/room_setting2/is_cs_advanced/language/voice_id. Team = tcp.RoomGroupInfo{id, name, abbr_name, members(List<RoomPlayerInfo>)}. Member = tcp.RoomPlayerInfo{account_id, nickname, ready(bool), role(ERoom::PlayerRole), group_id, head_pic, rank, cs_rank, ...}.**
  - evidence: func_query tcp::RoomInfo getters: get_id@0x4b51380, get_owner@0x4b51398/set_owner@0x4b513a0, get_code@0x4b51408, get_owner_online@0x4b51548, get_groups@0x4b51410, get_spectators@0x4b51418, get_state@0x4b513f8. RoomGroupInfo get_members@0x4b51250. RoomPlayerInfo get_account_id@0x4b51b24/get_ready@0x4b51b5c/get_role@0x4b51b80.
  - protos: tcp.RoomInfo (owner=3,code=10,owner_online=30); tcp.RoomGroupInfo; tcp.RoomPlayerInfo
  - files: protocol/protos/tcp.proto:2343; protocol/protos/tcp.proto:2331
  - confidence: high
- **There is NO dedicated transfer-host subcmd. Host/owner is modeled purely as RoomInfo.owner (uint64 account-id, set_owner @0x4b513a0); ownership/role changes are expressed via RequestSwitchSeat (subcmd 20, ERoom::PlayerRole) and the server rebroadcasts via ROOMINFO(12)/SWITCHSEAT_NTF(21). Client role of self = ECustomRoomRole (Owner=2).**
  - evidence: No RequestTransfer* method exists in COW::UIModelCustomRoom (full func_query enumerated). RoomInfo has set_owner but no client request sets it directly; only SwitchSeat carries role. set_MyRoomRole(ECustomRoomRole) @0x319503c.
  - symbols: _ZN3tcp8RoomInfo9set_ownerEm @0x4b513a0; _ZN3COW17UIModelCustomRoom14set_MyRoomRoleEN3COW15ECustomRoomRoleE @0x319503c
  - cmds: subcmd=20 SWITCHSEAT; subcmd=21 SWITCHSEAT_NTF; subcmd=12 ROOMINFO
  - confidence: medium
- **OUR proto is already complete and matches the client bit-for-bit; only the Node TCP tier lacks room wiring. tcp.proto defines Proto_ROOM=14 (line 1086), the full CustomRoom subcmd enum (ROOMLIST=1..COUNTDOWN_NTF=40) + ErrCode, and all Room* messages (RoomCreateReq@2289, RoomChangeReq@2260, RoomInfo@2343, RoomJoinReq@2401, RoomKickNtf, RoomDismissNtf, RoomJoinNtf, RoomStateNtf, RoomListReq/Res, RoomInfoReq, RoomInviteReq, RoomChangeAvailableMapsReq/Ntf).**
  - evidence: Read protocol/protos/tcp.proto lines 1086,1126-1165 (subcmd numbers identical to IDA), 2217-2415 (messages).
  - protos: tcp.EProtocol.Proto ROOM=14; tcp CustomRoom subcmd enum; tcp.Room* messages
  - files: protocol/protos/tcp.proto
  - confidence: high

**Our hooks:**
- src/tcp/router.js — register(protocol,subcmd,fn)/dispatch() keys handlers by `${protocol}:${subcmd}`; add handler modules under src/tcp/handlers/ exporting { protocol: 14 (EProtocol.ROOM), subcmd: N, reqType:'tcp.RoomCreateReq'|..., resType:'tcp.RoomInfo'|..., resCmd (for *_NTF replies routed under a different subcmd), handler(reqObj,ctx) }
- src/tcp/protocol.js — currently exposes only EProtocol/EFriend/EPresence/EStats/EMatchmaking via enumValues(); add `ECustomRoom: enumValues('tcp.ECustomRoom.Proto')` (the CustomRoom subcmd enum, tcp.proto ~line 1126) and `ECustomRoomErr: enumValues('tcp.ECustomRoom.ErrCode')` so room handlers reference named subcmds/error codes (mirrors EMatchmaking)
- protocol/protos/tcp.proto — Proto_ROOM=14 (line 1086); CustomRoom subcmd enum ROOMLIST=1..COUNTDOWN_NTF=40 (lines ~1126-1165); ErrCode SUSS=0..WORKSHOP_UPDATED=42 (lines 1167-1210); messages RoomCreateReq(2289)/RoomChangeReq(2260)/RoomInfo(2343,owner=3,code=10,owner_online=30)/RoomJoinReq(2401,code=2)/RoomKickNtf/RoomDismissNtf/RoomJoinNtf/RoomStateNtf/RoomListReq — all reqTypes ready to wire; no proto changes needed
- dispatch() reply routing: server replies/notifies must use MessageNotify.subCmd = the *_NTF value (e.g. START->handled S2C via ROOMSTATE_NTF=17/COUNTDOWN_NTF=40; SETREADY(24) broadcast->SETREADY_NTF(25); OWNERSWITCHGROUP(36)->OWNERSWITCHGROUP_NTF(37)); room-update refresh to all members = subcmd 12 ROOMINFO carrying tcp.RoomInfo

## Clash-Squad custom-room settings schema (client RE)

A host's Clash-Squad custom-room settings are carried entirely inside tcp.RoomInfo, split across two uint32 BITFIELDS plus one bytes blob: room_setting (field 18) packed per COW.ECustomRoomSetting, room_setting2 (field 19) packed per COW.ECustomRoomSetting2, is_cs_advanced (field 21) + cs_advanced_setting (field 22, a raw BinaryWriter blob — NOT protobuf) for the advanced CS economy (per-round money table + per-weapon shop prices), and level_visual_style (field 17) for weather. Simple toggles (UnlimitedAmmo, NoSkill=character-skills-off, FriendDmg=friendly-fire, NoHud, NoPowerGun=gun-attribute-lock, etc.) are single bits; numeric picks (starting HP, EP, move speed, jump height, RoundNum=number-of-rounds, InitCoin=starting-money, Revive) are multi-bit INDEX ranges into server-config value tables, not raw values. The definitive reader that maps every setting to its exact bit range is COW.UIModelCustomRoom.GenerateCustomRoomGameSettingDict @0x31a57b0; in-match the flags are consumed via COW.GamePlay.KPDMJKOEHEE.LLEOKLJJPJD(message.JCPLHHBPKPC) @0x19803c4 which reads room_setting(=GPKHLEAADHL)/room_setting2(=OJIHIKIAFAI) off the delivered match settings message into UIModelMatch. The authoritative enum definitions already exist in our repo at protocol/protos.bak/COW.proto and protos.bak/proto.proto (dropped from the active protos/).

- **All CS custom-room settings ride in tcp.RoomInfo via two uint32 bitfields, a bool, and a bytes blob (opaque at the proto level).**
  - evidence: tcp.proto RoomInfo: 'uint32 room_setting = 18; uint32 room_setting2 = 19; bool is_cs_advanced = 21; bytes cs_advanced_setting = 22;' (+ 'uint32 level_visual_style = 17' = weather)
  - symbols: tcp::RoomInfo::get_room_setting@0x4b51478; tcp::RoomInfo::get_room_setting2@0x4b51488; tcp::RoomInfo::get_level_visual_style@0x4b51468
  - protos: RoomInfo.room_setting=18; RoomInfo.room_setting2=19; RoomInfo.is_cs_advanced=21; RoomInfo.cs_advanced_setting=22; RoomInfo.level_visual_style=17
  - files: protocol/protos/tcp.proto:2343-2375
  - confidence: high
- **room_setting (field 18) bit layout = COW.ECustomRoomSetting mask enum: UnlimitedAmmo=2, NoFallingDamage=4, NoLoadout=8, NoAirdrop=16, NoSkill=32 (character skills off), NoVehicle=64, PlayerHP=bits8-10, PlayerEP=bits11-13, PlayerSpeed=bits14-16, DropList=bits17-20, PlayerJumpHeight=bits21-23, RoundNum=bits25-26, InitCoin=bits27-28, NoPowerGun=0x20000000 (gun-attribute lock), HideEnemyCloth=0x40000000, HideKillInfo=1.**
  - evidence: COW.proto enum ECustomRoomSetting values (mask form) match proto.proto CustomRoomSetting (index form) AND the exact GetRoomSettingValue()/GetSettingString() masks read in GenerateCustomRoomGameSettingDict: HP=GetRoomSettingValue(rs,0x100,0x400); EP=(rs,0x800,0x2000); Speed=(rs,0x4000,0x10000); Jump=(rs,0x200000,0x800000); RoundNum=(rs,0x2000000,0x4000000); InitCoin=(rs,0x8000000,0x10000000); NoPowerGun=GetSettingString(rs,0x20000000); HideEnemyCloth=(rs,0x40000000)
  - symbols: COW::UIModelCustomRoom::GenerateCustomRoomGameSettingDict@0x31a57b0; COW::UIModelCustomRoom::GetRoomSettingValue@0x319d1b0
  - protos: ECustomRoomSetting.UnlimitedAmmo=2; ECustomRoomSetting.NoSkill=32; ECustomRoomSetting.NoPowerGun=536870912; ECustomRoomSetting.PlayerHP_Start=256..End=1024; ECustomRoomSetting.RoundNum_Start=33554432..End=67108864; ECustomRoomSetting.InitCoin_Start=134217728..End=268435456; CustomRoomSetting.PLAYERHP_START=8..END=10; CustomRoomSetting.ROUNDNUM_START=25..END=26; CustomRoomSetting.INITCOIN_START=27..END=28; CustomRoomSetting.NOPOWERGUN=29
  - files: protocol/protos.bak/COW.proto:731-757; protocol/protos.bak/proto.proto:2070-2094
  - confidence: high
- **room_setting2 (field 19) bit layout = COW.ECustomRoomSetting2: NoUAV=1, NoBomb=2, Replay=4, NoZeppelin=8, NoHud=16, FriendDmg=32 (friendly fire), FightClubRoundNum=bits6-7, ReviveSwitch=bits8-10 (revive/respawn rule), InGameChat=2048, ShopFlow=4096, UseRandomMap=8192, NoAuxAim=16384 (aim-assist off).**
  - evidence: COW.proto ECustomRoomSetting2 mask enum matches GenerateCustomRoomGameSettingDict reads: NoUAV=GetSettingString(rs2,0x1); NoBomb=(rs2,0x2); NoZeppelin=(rs2,0x8); NoHud=(rs2,0x10); FriendDmg=(rs2,0x20); Revive=GetRoomSettingValue(rs2,0x100,0x400); FightClubRound=(rs2,0x40,0x80); ShopFlow=IsRoomSettingsTrue(rs2,0x1000); UseRandomMap=(rs2,0x2000); NoAuxAim=(rs2,0x4000); InGameChat=(rs2,0x800)
  - symbols: COW::UIModelCustomRoom::GenerateCustomRoomGameSettingDict@0x31a57b0
  - protos: ECustomRoomSetting2.NoUAV=1; ECustomRoomSetting2.NoBomb=2; ECustomRoomSetting2.NoZeppelin=8; ECustomRoomSetting2.NoHud=16; ECustomRoomSetting2.FriendDmg=32; ECustomRoomSetting2.FightClubRoundNum_Start=64..End=128; ECustomRoomSetting2.ReviveSwitchStart=256..End=1024; ECustomRoomSetting2.InGameChat=2048; ECustomRoomSetting2.ShopFlow=4096; ECustomRoomSetting2.UseRandomMap=8192; ECustomRoomSetting2.NoAuxAim=16384
  - files: protocol/protos.bak/COW.proto:759-775
  - confidence: high
- **The 26 host-facing settings and their loc titles are enumerated by InitCustomRoomSettingTitle via the COW.UIModelCustomRoom.ECustomRoomGameSetting enum: Revive(T_29_QX_ROOM_REVIVESET), HP(T_16_Z_ROOM_HP), EP, MoveSpeed, JumpHeight, Weather, UnLimitedAmmo, NoFallDamage, NoLoadOut, NoAirDrop, NoSkill(T_16_Z_ROOM_SKILL), NoVehicles, NoPowerGun(T_20_Z_R_GUN_PROPERTY = gun-attribute limit), NoUAV, NoBomb, NoZeppelin, HideEnemyCloth, NoHud, FriendDmg(T_28_Z_R_FRDMG = friendly fire), CSRound(T_20_S_CSROOM_ROUND = round count), CSInitCoin(T_20_S_CSROOM_COIN = starting money), FightClubRound, ShopFlow, UseRandomMap, NoAuxAim, InGameChat.**
  - evidence: InitCustomRoomSettingTitle builds Dictionary<ECustomRoomGameSetting,string> with 26 LocManager.DoLoc keys keyed by ECustomRoomGameSetting__Enum_{Revive,HP,EP,MoveSpeed,JumpHeight,Weather,UnLimitedAmmo,NoFallDamage,NoLoadOut,NoAirDrop,NoSkill,NoVehicles,NoPowerGun,NoUAV,NoBomb,NoZeppelin,HideEnemyCloth,NoHud,FriendDmg,CSRound,CSInitCoin,FightClubRound,ShopFlow,UseRandomMap,NoAuxAim,InGameChat}
  - symbols: COW::UIModelCustomRoom::InitCustomRoomSettingTitle@0x31a4b20
  - confidence: high
- **Numeric settings (HP/EP/Speed/Jump/RoundNum/InitCoin/Revive) store a small INDEX in the bitfield, not the raw value; the actual value is resolved from server config via RoomCreateRuleDataManager lookup tables.**
  - evidence: OnRoundNumSelected reads RoundNumConfigInfo.index (v8) then SetRoomSettingValue(RoundNum_Start,RoundNum_End,v8); GenerateCustomRoomGameSettingDict resolves display via RoomCreateRuleDataManager.GetRoundNumTxtByKey/GetInitCoinTxtByKey/GetHPTxtByKey/GetEPTxtByKey/GetSpeedTxtByKey/GetJumpHeightTxtByKey/GetReviveSwitchTxtByKey(index)
  - symbols: COW::UICreateRoomController::OnRoundNumSelected@0x2949274; COW::UICreateRoomController::OnInitCoinSelected@0x2949650; COW::UICreateRoomController::OnHPSelected@0x2947b44; COW::RoomCreateRuleDataManager::GetRoundNumTxtByKey@0x2978e44; COW::RoomCreateRuleDataManager::GetInitCoinTxtByKey@0x2978fcc
  - confidence: high
- **cs_advanced_setting (field 22) is a raw little-endian BinaryWriter blob (NOT protobuf) for advanced-CS economy: [u32 shopItemCount][shopItemCount x {u32 itemId, u32 price}][u32 ecoCount][ecoCount x {u32 itemId, u32 value}][u32 roundCount][roundCount x u16 perRoundCoin]. Disabled/banned weapons are simply omitted from the shop-item pairs (check-dict==false).**
  - evidence: GenerateADCSSettingBytes: Write(SelectedShopItemCnt); loop m_AdCSShopSettingValueDic writing key+value only where m_AdCSShopSettingCheckDic[key]==true; Write(ecoCount); loop writing key+value for entries NOT in check-dic; Write(_AdCSEcoRound); loop Write_10(m_AdCSEcoRoundValueDic[i]); debug string '@qhz GenerateADCSSettingBytes itemCnt: X => ecoCnt: Y => roundCnt: Z => bufferSize'
  - symbols: COW::UIModelCustomRoom::GenerateADCSSettingBytes@0x31983c0; COW::UIModelCustomRoom::GetCSRoundValueByIndex@0x31a4270; COW::UIModelCustomRoom::InitDefaultCSSettingFromConfig@0x31957ec
  - confidence: high
- **Advanced-CS defaults come from GameVarDef: CustomRoomCSInitCoin (a ';'-separated string parsed into the per-round coin table), CustomRoomCSMaxRound (pads remaining rounds with the last value), CustomRoomCSShopCostInterval (divisor turning shop price into eco units).**
  - evidence: InitDefaultCSSettingFromConfig: reads GameVarDef.CustomRoomCSInitCoin, String.Split(';'), ConvertAll<int> into m_AdCSEcoRoundValueDic; while(m<GameVarDef.CustomRoomCSMaxRound) pad with last value; shop price = StickerSprite.m_Button / CustomRoomCSShopCostInterval
  - symbols: COW::UIModelCustomRoom::InitDefaultCSSettingFromConfig@0x31957ec; COW::GameVarDef::CustomRoomCSInitCoin; COW::GameVarDef::CustomRoomCSMaxRound; COW::GameVarDef::CustomRoomCSShopCostInterval
  - confidence: high
- **When is_cs_advanced/ADCSEnabled is set, the round count and starting money come from the cs_advanced_setting blob (AdCSEcoRound + GetCSRoundValueByIndex(0)) INSTEAD of the room_setting RoundNum/InitCoin bit indices.**
  - evidence: GenerateCustomRoomGameSettingDict: 'if(this->_ADCSEnabled_k__BackingField){ roundTxt = _AdCSEcoRound; initCoinTxt = GetCSRoundValueByIndex(0);} else { GetRoundNumTxtByKey(rsRoundIdx); GetInitCoinTxtByKey(rsInitCoinIdx);}'
  - symbols: COW::UIModelCustomRoom::GenerateCustomRoomGameSettingDict@0x31a57b0; COW::UIModelCustomRoom::GetCSRoundValueByIndex@0x31a4270
  - confidence: high
- **In-match, the settings arrive as message.JCPLHHBPKPC {List<message.FFKKHGEMNAN> JHNBBOJBPAJ; uint32 GPKHLEAADHL=room_setting; uint32 OJIHIKIAFAI=room_setting2}, read by COW.GamePlay.KPDMJKOEHEE.LLEOKLJJPJD which sets UIModelMatch.IsCustomRoomSettingFriendDmg (rs2 bit0x20), .IsCustomRoomSettingUnlimitedAmmo (rs bit0x2), .IsCustomRoomSettingNoHud (rs2 bit0x10) and dispatches GLOBALEVENT_NO_HUD.**
  - evidence: LLEOKLJJPJD: HasFlag(LHCHNFGKLHD->OJIHIKIAFAI,0x20)->set_IsCustomRoomSettingFriendDmg; HasFlag(GPKHLEAADHL,2)->set_IsCustomRoomSettingUnlimitedAmmo; HasFlag(OJIHIKIAFAI,0x10)->set_IsCustomRoomSettingNoHud; struct JCPLHHBPKPC size 40: GPKHLEAADHL@0x20 u32, OJIHIKIAFAI@0x24 u32, JHNBBOJBPAJ@0x18 List<FFKKHGEMNAN>
  - symbols: COW::GamePlay::KPDMJKOEHEE::LLEOKLJJPJD@0x19803c4; COW::UIModelMatch::set_IsCustomRoomSettingFriendDmg@0x1ad1198; COW::UIModelMatch::set_IsCustomRoomSettingUnlimitedAmmo@0x1ad115c; COW::UIModelMatch::set_IsCustomRoomSettingNoHud@0x1ad1184; COW::GamePlay::KEPDHPAAHGP::PCIJJECCOEN@0x1b34b28
  - protos: JCPLHHBPKPC.JHNBBOJBPAJ (List<FFKKHGEMNAN>); JCPLHHBPKPC.GPKHLEAADHL (uint32=room_setting); JCPLHHBPKPC.OJIHIKIAFAI (uint32=room_setting2)
  - confidence: high
- **The consumed in-match flags live on UIModelMatch as booleans read by gameplay/HUD: IsCustomRoomSettingNoSkill, UnlimitedAmmo, FriendDmg, NoHud, HideCloth, HideKillInfo, InGameChatOpen, NoAuxAim.**
  - evidence: COW::UIModelMatch get_/set_ pairs: get_IsCustomRoomSettingHideKillInfo@0x1ad1118, NoSkill@0x1ad112c, HideCloth@0x1ad1140, UnlimitedAmmo@0x1ad1154, NoHud@0x1ad117c, FriendDmg@0x1ad1190, InGameChatOpen@0x1ad11a4, NoAuxAim@0x1ad11b8
  - symbols: COW::UIModelMatch::get_IsCustomRoomSettingNoSkill@0x1ad112c; COW::UIModelMatch::get_IsCustomRoomSettingUnlimitedAmmo@0x1ad1154; COW::UIModelMatch::get_IsCustomRoomSettingFriendDmg@0x1ad1190; COW::UIModelMatch::get_IsCustomRoomSettingNoAuxAim@0x1ad11b8
  - confidence: high
- **Host write path: UICreateRoomController packs each toggle into room_setting via GCommon.BitArray.AddFlag/RemoveFlag (single bits) and numeric picks via SetRoomSettingValue(start,end,value) which spreads value across the bit range; RefreshGameSettingInfo re-emits all values.**
  - evidence: SetRoomSetting(u32,bool) -> BitArray.AddFlag/RemoveFlag(m_RoomSetting,setting); SetRoomSettingValue(start,end,value){ for(;start<=end;value>>=1){ SetRoomSetting(start,value&1); start*=2; } }; IsRoomSettingsTrue -> BitArray.HasFlag
  - symbols: COW::UICreateRoomController::SetRoomSetting@0x294ca1c; COW::UICreateRoomController::SetRoomSettingValue@0x294683c; COW::UICreateRoomController::GetRoomSettingValue@0x294cb30; COW::UICreateRoomController::RefreshGameSettingInfo@0x2946454; GCommon::BitArray::AddFlag@0x33dd418
  - confidence: high
- **The authoritative enum text already exists in our repo (protos.bak) but was dropped from the active protos/, and neither room_setting nor cs_advanced_setting is parsed anywhere in the Go match-server yet.**
  - evidence: protocol/protos.bak/COW.proto:731 enum ECustomRoomSetting + :759 ECustomRoomSetting2; protos.bak/proto.proto:2070 CustomRoomSetting + :2096 CustomRoomSetting2; grep of match-server/**/*.go finds NO room_setting/cs_advanced references (only udp_cmds.txt RUDP_S2C_GLOBAL_OBEVENT_ROOMSETTING_NTF=644)
  - files: protocol/protos.bak/COW.proto:731-775; protocol/protos.bak/proto.proto:2070-2099; protocol/udp_cmds.txt:1097
  - confidence: high

**Our hooks:**
- match-server/cssync.go:13 const maxRound = 7  -> would be overridden by RoomInfo.room_setting RoundNum bits25-26 (ECustomRoomSetting.RoundNum_Start=0x2000000..End=0x4000000), or by cs_advanced_setting AdCSEcoRound when is_cs_advanced
- match-server/cssync.go:14 roundsToWin = (maxRound+1)/2 // 4 -> derived from round count
- match-server/player.go:25 const maxHP = 200 -> RoomInfo.room_setting PlayerHP bits8-10 (ECustomRoomSetting.PlayerHP_Start=256..End=1024, index into HP table)
- match-server/purchase.go:15 const startingCoins = 500 -> RoomInfo.room_setting InitCoin bits27-28 (ECustomRoomSetting.InitCoin_Start=0x8000000..End=0x10000000), or cs_advanced_setting per-round coin table when is_cs_advanced
- match-server/ep.go:15 const maxEP = 200 -> RoomInfo.room_setting PlayerEP bits11-13 (ECustomRoomSetting.PlayerEP_Start=2048..End=8192)
- match-server/results.go:18 settleWinCoins=200 / deathmatch.go per-round kill coin credit -> per-round money economy driven by cs_advanced_setting round table
- NO EXISTING HOOK for friendly-fire (room_setting2 FriendDmg=32), NoSkill/character-skills-off (room_setting NoSkill=32), UnlimitedAmmo (room_setting=2), NoPowerGun/gun-attribute-lock (room_setting=0x20000000), NoAuxAim, Revive rule (room_setting2 bits8-10): these are new match-server handlers that must key off room_setting/room_setting2 bits once the lobby plumbs RoomInfo into the match join proto (client match-side struct = message.JCPLHHBPKPC)

**Open questions:**
- Exact proto FIELD NUMBERS of message.JCPLHHBPKPC (the in-match settings message) are not confirmed — struct offsets imply JHNBBOJBPAJ(list) then GPKHLEAADHL(u32) then OJIHIKIAFAI(u32); it is not present in our extracted protos/. Needs the serializer/parser decompile to fix the wire tags.
- What message.FFKKHGEMNAN (the repeated element JHNBBOJBPAJ in JCPLHHBPKPC) carries — likely the advanced-CS per-weapon shop-price / per-round eco entries delivered to the match; confirm by decompiling its serialize path.
- The concrete value tables behind the bit INDICES (HP, EP, speed, jump, RoundNum, InitCoin, Revive) come from RoomCreateRuleDataManager / RoomCreateRuleRes config (server CSV), not the client binary — the index->value mapping must be sourced from game config, not hard-coded.
- DropList (room_setting bits17-20) and AccTotalStats (bit24) appear BR-oriented; ShopFlow/UseRandomMap/FightClubRound are FightClub-mode settings — confirm which subset the CS-Ranking/CS-Custom UI actually exposes vs. ignores.
- How the TCP lobby converts RoomInfo.room_setting/cs_advanced_setting into the match-server payload (which command / join field) — our Go match-server currently ignores it entirely.

## Room / CustomRoom protocol + settings inventory (proto + reference server)

The ROOM protocol is EProtocol=14 in every source (reference tcp.proto, our tcp.proto, reference tcpp.py runtime). OUR protobuf is FAR more complete than the reference: all 33 room messages and all room enums load and round-trip through src/protocol/protos.js today (0 misses) — RoomInfo, RoomCreateReq/ChangeReq, RoomJoinReq/Ntf, RoomListReq/Res, RoomPlayerInfo, RoomSwitchSeat/Invite/SetReady/CountDown, plus the CS-specific bitmask enums ECustomRoomSetting/ECustomRoomSetting2 in COW.proto and the cs_advanced_setting bytes blob. The reference server (tcpp.py) DID implement a working CustomRoom flow (list/create/join/spectate/leave/kick/start/info/change) BUT encodes it as ad-hoc JSON (json.dumps), NOT protobuf, with field names that don't match the proto schema — so it is a logic/flow reference only, not a wire reference. OUR node stack has ZERO room handling: no TCP route registers protocol 14, and the only room code is an HTTP GetRoomList.js stub returning {room_list:[]} (mirroring htpp.py's empty stub). Everything needed to decode/encode rooms exists; nothing is wired.

- **ROOM is EProtocol value 14 in all sources; the reference server dispatches CMD=14 to handle_custom_room.**
  - evidence: reference tcp.proto:362 EProtocol_Proto_ROOM=14; our tcp.proto:1086 Proto_ROOM=14; tcpp.py:792 CustomRoom=14 and tcpp.py:4501 -> handle_custom_room(session,data_json) for CMD 14.
  - protos: EProtocol.Proto.Proto_ROOM=14
  - cmds: 14
  - files: original_to_read/tcp.proto; original_to_read/tcpp.py; protocol/protos/tcp.proto
  - confidence: high
- **The room SUBCOMMAND enum is ERoom.Proto. Reference tcp.proto defines 25 members (0-25); OUR tcp.proto defines 40 members (0-40) with extra CS/workshop/admin ops; reference tcpp.py's RUNTIME enum ECustomRoomSubCmd only goes to 17.**
  - evidence: reference tcp.proto:235-262 ERoom_Proto NONE..SETREADY_NTF=25; our tcp.proto:1123-1166 ERoom.Proto adds REAL_CREATE=26, ADMIN_SET_ICON=27, ADMIN_DISMISS_ROOM=28, ROOM_CREATE_RULE=32, CHANGEAVAILABLEMAPS=33/34, OWNERSWITCHGROUP=36/37, COUNTDOWN=39/40; tcpp.py:868-887 ECustomRoomSubCmd stops at ROOMSTATE_NTF=17.
  - protos: ERoom.Proto (tcp.RoomProto subcmds 0-40)
  - cmds: 1 ROOMLIST; 2 CREATE; 3 JOIN; 4 SPECTATE; 5 JOIN_NTF; 6 LEAVE; 7 LEAVE_NTF; 8 DISMISS_NTF; 9 KICK; 10 KICK_NTF; 11 START; 12 ROOMINFO; 13 CHANGE; 14 CHANGE_NTF; 15 DROPMATCH/DROPOMATCH; 16 MATCHMAKINGSUSS_NTF; 17 ROOMSTATE_NTF; 20 SWITCHSEAT; 22 INVITE; 24 SETREADY; 39/40 COUNTDOWN
  - files: protocol/protos/tcp.proto; original_to_read/tcp.proto; original_to_read/tcpp.py
  - confidence: high
- **Room error/type/role enums exist and are richer in ours. ERoom.ErrCode: reference 0-28 (tcp.proto:264-293), ours 0-42 (tcp.proto:1167-1211). Reference tcpp.py runtime ECustomRoomErrCode stops at 16, ECustomRoomRole = PLAYER=0/SPECTATOR=1/OWNER=2.**
  - evidence: our tcp.proto:1167-1211 ERoom.ErrCode NOROOM=1..WORKSHOP_UPDATED=42; ERoom.Type:1212-1220 (CASUAL=1,LEAGUE_NORMAL=2,LEAGUE_BATCH=6,WEREWOLVES=7,WORKSHOP=8,RUSHING_PETS=9); ERoom.PlayerRole:1229 MEMBER=1/SPECTATOR=2; ERoom.State:1240 IDLE=0/INGAME=1; ERoom.DismissReason:1244 NORMAL=1/OFFLINE=2/TIMEOUT=3/ADMIN=4; ERoom.UpdateRoomEvent:1251-1263; tcpp.py:889-913 ECustomRoomErrCode/ECustomRoomRole.
  - protos: ERoom.ErrCode; ERoom.Type; ERoom.PlayerRole; ERoom.State; ERoom.PlayerState; ERoom.DismissReason; ERoom.UpdateRoomEvent; ERoom.TabType
  - files: protocol/protos/tcp.proto; original_to_read/tcp.proto; original_to_read/tcpp.py
  - confidence: high
- **Room SETTINGS are bitmask flags packed into uint32 fields room_setting/room_setting2 (plus a cs_advanced_setting bytes blob), defined by COW.ECustomRoomSetting (24 flags) and COW.ECustomRoomSetting2 (14 flags) in OUR protos only. The reference server has no settings enum.**
  - evidence: COW.proto:731-757 ECustomRoomSetting (HideKillInfo=1,UnlimitedAmmo=2,NoFallingDamage=4,NoLoadout=8,NoAirdrop=16,NoSkill=32,NoVehicle=64, PlayerHP/EP/Speed/Jump ranges, RoundNum_Start=33554432/End=67108864, InitCoin_Start=134217728/End=268435456, NoPowerGun=536870912, HideEnemyCloth=1073741824); COW.proto:759-775 ECustomRoomSetting2 (NoUAV=1,NoBomb=2,Replay=4,NoZeppelin=8,NoHud=16,FriendDmg=32,FightClubRoundNum,ReviveSwitch,InGameChat=2048,ShopFlow=4096,UseRandomMap=8192,NoAuxAim=16384). Carried by RoomCreateReq.room_setting=14 & room_setting2=18 & cs_advanced_setting(bytes)=22 (tcp.proto:2303/2307/2311); RoomInfo.room_setting=18/room_setting2=19/cs_advanced_setting=22 (tcp.proto:2361-2365).
  - protos: COW.ECustomRoomSetting; COW.ECustomRoomSetting2; RoomCreateReq.room_setting=14; RoomCreateReq.room_setting2=18; RoomCreateReq.cs_advanced_setting=22; RoomInfo.room_setting=18; RoomInfo.cs_advanced_setting=22
  - files: protocol/protos/COW.proto; protocol/protos/tcp.proto
  - confidence: high
- **OUR RoomInfo (tcp.proto:2343) has 31 fields incl. CS/workshop extensions the reference RoomInfo (tcp.proto:872, 16 fields) lacks: level_visual_style, room_setting, room_setting2, is_cs_advanced, cs_advanced_setting, werewolves params, language, workshop, voice_id.**
  - evidence: our tcp.proto:2343-2375 RoomInfo{id,name,owner,map_id,game_mode,group_mode,max_member_num,max_spectator_num,state,code,groups[],spectators[],enable_death_spectate,room_type(SimpleJSON.Type),enable_group_icon,match_times,level_visual_style=17,room_setting=18,room_setting2=19,enable_emulator_check=20,is_cs_advanced=21,cs_advanced_setting=22,werewolves_room_param=23..25,contestant_role_check=26,workshop=27,enough_room_card=29,owner_online=30,voice_id=31}; reference tcp.proto:872-889 stops at match_times=16.
  - protos: tcp.RoomInfo (31 fields); tcp.RoomGroupInfo{id,name,members[],abbr_name}; tcp.RoomPlayerInfo{group_id,account_id,nickname,emulator_score,head_pic,ready,banner_id,role,available_maps[],pin_id,using_version,rank,ranking_points,cs_rank,cs_ranking_points,voice_id,peak_rank_pos,cs_peak_rank_pos}
  - files: protocol/protos/tcp.proto; original_to_read/tcp.proto
  - confidence: high
- **Full request/notify message set exists in OUR tcp.proto: RoomListReq/Res, RoomCreateReq(33 fields), RoomJoinReq, RoomSpectateReq, RoomJoinNtf, RoomLeaveReq/Ntf, RoomKickReq/Ntf, RoomChangeReq, RoomInfoReq, RoomStateNtf, RoomSwitchSeatReq, RoomInviteReq/Ntf, RoomSetReadyReq/Ntf, RoomDismissNtf, RoomStartReq, RoomCountDownReq/Ntf, RoomChangeAvailableMapsReq/Ntf, RoomOwnerSwitchGroupReq, RoomIDReq, JoinRoomPlayerInfo, RoomCardInfo, CustomRoomSinglePlayerMatchStats.**
  - evidence: tcp.proto grep: RoomBasicInfo:2217, RoomCardInfo:2242, RoomChangeAvailableMapsNtf:2248/Req:2254, RoomChangeReq:2260, RoomCountDownNtf:2280/Req:2284, RoomCreateReq:2289, RoomDismissNtf:2325, RoomGroupInfo:2331, RoomIDReq:2338, RoomInfo:2343, RoomInfoReq:2377, RoomInviteNtf:2382/Req:2392, RoomJoinNtf:2396/Req:2401, RoomKickNtf:2415/Req:2420, RoomLeaveNtf:2425/Req:2430, RoomListReq:2434/Res:2441, RoomOwnerSwitchGroupReq:2445, RoomPlayerInfo:2454, RoomSetReadyNtf:2475/Req:2480, RoomSpectateReq:2485, RoomStartReq:2496, RoomStateNtf:2500, RoomSwitchSeatReq:2505; JoinRoomPlayerInfo:1694; CustomRoomSinglePlayerMatchStats:336.
  - protos: RoomCreateReq(33 fields incl workshop_id=28,workshop_code=31,ping_list=15,available_maps=16,reopen_id=17,creater_role=20,contestant_role_check=26); RoomJoinReq(room_id,code,group_id,is_solo,players[JoinRoomPlayerInfo],group_name,inviter_account_id,ping_list,available_maps,room_type,group_abbr_name); RoomListReq(room_id,room_type,game_modes[COW.GameMode],room_tab_type[COW.TabType])
  - files: protocol/protos/tcp.proto
  - confidence: high
- **Reference server IMPLEMENTED these room subcmds (real handlers): ROOMLIST(1), CREATE(2), JOIN(3), SPECTATE(4), LEAVE(6), KICK(9), START(11), ROOMINFO(12), CHANGE(13). All other subcmds fall through to 'SubCMD não implementado'. START does the UDP handoff.**
  - evidence: tcpp.py:3547-3583 handle_custom_room dispatch table; handlers at 3585 room_list, 3608 create, 3656 join, 3710 spectate, 3746 leave, 3783 kick, 3813 start, 3877 info, 3901 change; tcpp.py:3583 else-branch logs 'não implementado'. tcpp.py:5121 disconnect cleanup calls handle_room_leave. START (3855-3870) sends MATCHMAKINGSUSS_NTF(16) with {server_addr,secret,match_id,prepare_token,map_id,game_mode,match_mode:EGAMETYPE.Room}.
  - cmds: 1; 2; 3; 4; 6; 9; 11; 12; 13; 16
  - files: original_to_read/tcpp.py
  - confidence: high
- **Reference server STUBBED/NEVER-HANDLED subcmds: SWITCHSEAT(20), INVITE(22), SETREADY(24) are not even defined in its runtime ECustomRoomSubCmd enum (which stops at 17); DROPMATCH(15), ROOMSTATE_NTF(17) have no handler. So seat-switching, invites, ready-state and room-list-broadcast are absent in the reference.**
  - evidence: tcpp.py:868-887 ECustomRoomSubCmd max member ROOMSTATE_NTF=17; dispatch (3555-3581) has no branch for 15/17/20/22/24; these route to the 3583 'não implementado' log.
  - cmds: 15; 17; 20; 22; 24; 25
  - files: original_to_read/tcpp.py
  - confidence: high
- **CRITICAL wire caveat: the reference room tier is JSON, not protobuf. tcpp.py send_packet encodes CMD=14 replies as json.dumps({cmd,data}) with ad-hoc field names (room_id, creator_id, creator_name, players) that do NOT match the protobuf RoomInfo schema (id, owner, groups[]). It is a flow/logic reference, not a wire-format reference. OUR stack is protobuf.**
  - evidence: tcpp.py:1376-1394 send_packet: payload=json.dumps({'cmd':subcmd,'data':json.dumps(data)}); header=struct.pack('>BBI',packet_cmd,1,len). All room handlers pass python dicts with keys room_id/creator_id/creator_name/players (e.g. 3699-3707 join, 3885-3895 info) vs proto RoomInfo{id,owner,groups}.
  - files: original_to_read/tcpp.py
  - confidence: high
- **Empirically, OUR src/protocol/protos.js resolves and round-trips ALL room types today: 33/33 messages OK, 13/13 enums OK, 0 miss; RoomInfo encode/decode verified. Types are decodable by bare name via lookup()/lookupEnum().**
  - evidence: Ran a node harness against src/protocol/protos.js: every one of RoomInfo/RoomBasicInfo/RoomPlayerInfo/RoomGroupInfo/RoomCreateReq/RoomJoinReq/RoomJoinNtf/RoomLeaveReq/RoomLeaveNtf/RoomListReq/RoomListRes/RoomInfoReq/RoomChangeReq/RoomKick*/RoomStart*/RoomStateNtf/RoomSwitchSeatReq/RoomInvite*/RoomSetReady*/RoomDismissNtf/RoomSpectateReq/JoinRoomPlayerInfo/RoomCardInfo/RoomCountDown*/RoomChangeAvailableMapsReq/RoomOwnerSwitchGroupReq/CustomRoomSinglePlayerMatchStats/GroupJoinRoomReq/GroupSpectateRoomReq returned OK; enums ERoom.Proto/ErrCode/Type/PlayerRole/State/PlayerState/DismissReason/UpdateRoomEvent/TabType + COW.ECustomRoomSetting/ECustomRoomSetting2/ECustomRoomRole + EProtocol.Proto all OK; RoomInfo roundtrip {id:5,name:x,owner:9,map_id:1,game_mode:1}.
  - files: src/protocol/protos.js; protocol/protos/tcp.proto; protocol/protos/COW.proto
  - confidence: high
- **Name-collision gotcha for the implementer: RoomBasicInfo and ERoom exist in BOTH tcp.proto and proto.proto with DIFFERENT shapes; lookup() returns the proto.proto variant (PACKAGES order ['proto','tcp',...]). Use fully-qualified 'tcp.RoomBasicInfo' to get the RoomListRes-referenced one.**
  - evidence: lookup('RoomBasicInfo') resolved to .proto.RoomBasicInfo (proto.proto:14407, 21 fields, field15=uint32 room_type,+code=17,+enough_room_card=20,+owner_online=21) vs tcp.proto:2217 (22 fields, field15=SimpleJSON.Type room_type,+owner_role=18,+is_cs_advanced=19,+language=20,+contestant_role_check=21,+room_setting=22). lookupEnum('ERoom.Type') resolved to .proto.ERoom (proto.proto:9687 is a DIFFERENT ERoom with nested Cmd{SEND_ALL_ROOMS,OK}); ERoom.Proto subcmd enum only exists as .tcp.ERoom.Proto. RoomListRes.room_list inside tcp.proto binds to .tcp.RoomBasicInfo by same-file scope.
  - protos: proto.RoomBasicInfo != tcp.RoomBasicInfo; proto.ERoom (Cmd enum) != tcp.ERoom (Proto/ErrCode/Type)
  - files: protocol/protos/proto.proto; protocol/protos/tcp.proto; src/protocol/protos.js
  - confidence: high
- **OUR node server has NO room handling wired. No TCP handler registers protocol 14; the only room code is HTTP GetRoomList.js, a stub returning {room_list:[]}, matching htpp.py's empty-bytes stub.**
  - evidence: src/tcp/handlers/ contains only GameOpeningInfo.js, MatchmakingCancel.js, MatchmakingDropMatch.js, MatchmakingStart.js, PresenceList.js (all protocol 3/MATCHMAKING or presence) — grep for protocol:14/ERoom in src/tcp returned no matches. src/handlers/GetRoomList.js:10-12 returns {room_list:[]}; htpp.py:6059-6062 handle_GetRoomList returns empty bytes. Router (src/tcp/router.js:22-45) keys handlers by (protocol,subcmd) loaded from src/tcp/handlers/*.js.
  - files: src/tcp/router.js; src/tcp/handlers/; src/handlers/GetRoomList.js; original_to_read/htpp.py
  - confidence: high

**Our hooks:**
- src/tcp/router.js register(protocol,subcmd,fn): a room tier drops in as new src/tcp/handlers/Room*.js modules exporting {protocol:14, subcmd: <ERoom.Proto value>, reqType, resType, handler} — the loader auto-registers them (router.js:28-45). No protocol-14 route exists yet.
- src/protocol/protos.js lookup('tcp.RoomInfo')/lookup('tcp.RoomCreateReq')/lookupEnum('tcp.ERoom.Proto') already resolve — encode replies against tcp.* fully-qualified names to avoid the proto.proto RoomBasicInfo/ERoom collisions.
- Room settings surface = RoomCreateReq.room_setting(field14)+room_setting2(field18)+cs_advanced_setting bytes(field22) and RoomChangeReq.room_setting(11)/room_setting2(12)/cs_advanced_setting(14); a custom-room settings override maps to COW.ECustomRoomSetting / COW.ECustomRoomSetting2 bit values.
- Match handoff constant: EMatch.MatchMode.ROOM=3 / reference EGAMETYPE.Room=3; START (ERoom.Proto=11) must emit MATCHMAKINGSUSS_NTF (subcmd 16) carrying server_addr/secret/match_id/prepare_token/map_id/game_mode/match_mode — reuse our existing dedicated matchmaker handoff (src/servers/matchmaker.js) that MatchmakingStart.js already drives for protocol 3.
- src/handlers/GetRoomList.js is the HTTP RoomListReq->RoomListRes stub ({room_list:[]}) — the TCP ROOMLIST(1) path is separate and unimplemented.
- PresenceList.js already models INROOM presence state (PresenceInfo.p) — room join/leave should update presence.

**Open questions:**
- Does the real FF 1.70.1 client accept JSON on protocol 14 (as reference tcpp.py sends) or does it require protobuf RoomInfo/RoomCreateReq? The proto schema's existence strongly implies protobuf; the reference's JSON+mismatched-field-names room tier may never have actually driven the retail client. Needs an IDA check of the client's protocol-14 deserialize path or a live capture.
- What is the exact byte layout of the cs_advanced_setting bytes blob (RoomCreateReq field 22 / RoomInfo field 22)? It is opaque bytes in the proto; the CS-advanced room config (round count, init coin, HP/EP/speed ranges beyond the bitmask) likely lives here and needs RE.
- Which ERoom.Proto subcmds the retail client actually sends for a CS custom room flow (CREATE/JOIN/SETREADY/START vs the newer REAL_CREATE=26, COUNTDOWN=39, CHANGEAVAILABLEMAPS=33) — the reference only exercised the classic 1-13 set.
- Header framing for protocol 14 in OUR stack: confirm our TCP gateway wraps ROOM replies in the same ProtoReq/MessageNotify envelope our matchmaking handlers use (router.js dispatch assumes ProtoReq.cmd=subcmd).

## OUR Go match-server (match-server/**): CS round system, economy, shop, loadout, config plumbing, and the per-match data channel a custom-room settings blob would ride in.

Every knob a custom room would override is a package-level Go const or a hardcoded literal, and the only per-match input channel today is the per-PLAYER prepare_token JWT (identity + cosmetics) with NO room-settings field. Round length is const maxRound=7 / roundsToWin=4 (cssync.go); the match-end decision is Match.endFight (match.go:762). Coin bonuses are BARE 500 literals in endFight (match.go:752-755) with NO named constants and NO per-loss bonus; startingCoins=500 (purchase.go:15). The shop is a static []message.ShopItem csShopItems (shop.go:17) + const csShopTitle. Player HP is const maxHP=200 (player.go:25); the starting loadout (USP data 3 + 2 medkits) is hardcoded in session.giveLoadout (cssync.go:97). All existing MATCH_* overrides are PROCESS-GLOBAL (config.go cfg, read once in loadConfig), NOT per-match, so per-room settings need a new per-Match channel that extends token.Claims (jwt.go) fed by matchmaker buildPrepareToken (src/tcp/matchmaker.js:70) and threaded onto *Match/*session.

- **Round count and rounds-to-win are compile-time constants, not per-match: maxRound=7 and roundsToWin=(maxRound+1)/2=4.**
  - evidence: const ( maxRound = 7; roundsToWin = (maxRound + 1) / 2 // 4 )
  - cmds: 901
  - files: match-server/cssync.go
  - confidence: high
- **The match decides it ENDED in Match.endFight: after scoring a round, if either team's score reaches roundsToWin OR the round index reaches maxRound, it sets m.winnerTeam and enters phMatchEndPause (fires cmd 103).**
  - evidence: match.go:762 `if m.teamScore[0] >= roundsToWin || m.teamScore[1] >= roundsToWin || m.round >= maxRound {` -> m.winnerTeam=winnerTeam; m.enter(now, phMatchEndPause, matchEndPause)
  - symbols: endFight (match.go:737)
  - cmds: 103
  - files: match-server/match.go
  - confidence: high
- **phMatchEndPause's edge calls Match.matchEnd(), which sends cmd 103 MatchEnd PER PLAYER (rank 1 to p.team==m.winnerTeam else 2), sets m.ended=true, then holds phMatchEnd matchStatsHold=20s before teardown.**
  - evidence: match.go:694-697 `case phMatchEndPause: m.matchEnd(); m.ended = true; m.enter(now, phMatchEnd, matchStatsHold)`; round.go:47-58 matchEnd loops m.players, rank from p.team==m.winnerTeam, sendDataLog(packet.CmdMatchEnd, message.MatchEnd(rank))
  - symbols: Match.matchEnd (round.go:47); phMatchEndPause (match.go:90)
  - cmds: 103
  - files: match-server/match.go; match-server/round.go
  - confidence: high
- **Which team WON a round is decided by survivorship in endFight: localWon = m.teamAlive(localTeamID) > 0; the surviving team's score increments (teamScore[0] local / teamScore[1] enemy).**
  - evidence: match.go:741-748 `localWon := m.teamAlive(localTeamID) > 0` then teamScore[0]++ or winnerTeam=enemyTeamID;teamScore[1]++
  - symbols: localTeamID=1 / enemyTeamID=2 (round.go:17-18); teamScore [2]uint8 (match.go:34)
  - files: match-server/match.go; match-server/round.go
  - confidence: high
- **COIN bonuses are all BARE 500 LITERALS in endFight with NO named constants: per-kill=500*roundKills; per-round participation=500*(round+1); per-WIN=+500 for winning-team members. NO per-loss bonus. Award added to p.coins and stored as p.award (PRI EarnedCoin).**
  - evidence: match.go:752-758 `aw := 500 * p.roundKills.Swap(0); aw += 500 * (uint32(m.round) + 1); if p.team == winnerTeam { aw += 500 }; p.coins += aw; p.award = aw`
  - symbols: session.coins uint32 (session.go:64); session.award uint32 (session.go:65); session.roundKills atomic.Uint32 (session.go:60)
  - files: match-server/match.go
  - confidence: high
- **roundKills (per-kill coin driver) is incremented in Match.creditDeath only for an enemy-team kill (killer!=0 and different team); environment/zone kills give no coins; reset each round via .Swap(0).**
  - evidence: deathmatch.go:142-149 creditDeath: `if killer != 0 && m.teamOf(killer) != m.teamOf(victim) { m.kills[killer]++; if ks := m.sessionByEntity(killer); ks != nil { ks.roundKills.Add(1) } }`; consumed at match.go:752
  - symbols: creditDeath (deathmatch.go:142)
  - files: match-server/deathmatch.go; match-server/match.go
  - confidence: high
- **Starting buy-phase coins are const startingCoins=500, seeded at match start in run() (s.coins) and in giveLoadout(resetCoins=true); unlimitedMoneyTest overrides both to 9999.**
  - evidence: purchase.go:15 `const startingCoins = 500`; match.go:356-360 `coins := uint32(startingCoins); if cfg.unlimitedMoneyTest { coins = 9999 }; s.coins = coins`; cssync.go:103-104 `if resetCoins { s.coins = startingCoins }`
  - symbols: startingCoins (purchase.go:15); sendStartingLoadout (cssync.go:87)
  - files: match-server/purchase.go; match-server/match.go; match-server/cssync.go
  - confidence: high
- **The shop catalogue is a single static package var csShopItems ([]message.ShopItem) of 22 entries (16 weapons Filter=1, armour/utility Filter=2, throwables Filter=4) with per-item ItemID/Price/Filter/Limitation; title is const csShopTitle. shopItemByID does a linear lookup. Sent verbatim via cmd 407 in sendCSShop -> message.CSShop(csShopItems, csShopTitle, false). A comment flags per-round re-pricing as TODO.**
  - evidence: shop.go:17 `var csShopItems = []message.ShopItem{ {ItemID:10,Price:500,Filter:1}(G18) ... {ItemID:4,Price:2100,Filter:1}(AWM), {ItemID:302,Price:400,Filter:2}, {ItemID:601,Price:200,Filter:4,Limitation:1}, {ItemID:1201,Price:300,Filter:4,Limitation:3}, {ItemID:709,Price:100,Filter:4,Limitation:2} }`; shop.go:7 csShopTitle; cssync.go:229-231 sendCSShop; shop.go:15-16 TODO round pricing
  - symbols: csShopItems (shop.go:17); csShopTitle (shop.go:7); shopItemByID (shop.go:45); sendCSShop (cssync.go:229)
  - cmds: 407; 408
  - files: match-server/shop.go; match-server/cssync.go; match-server/message/csshop.go
  - confidence: high
- **cmd 408 handleCSPurchase validates itemID/price against csShopItems via shopItemByID, deducts s.coins, enforces per-round limit only for the mushroom; unlimitedMoneyTest bypasses coin check/deduction. Item->slot maps (weaponAmmo, armorSlot, buildingSlot, ammoStack=30) are all hardcoded in purchase.go.**
  - evidence: purchase.go:141-224 handleCSPurchase: shopItemByID, `price := item.Price * qty`, `if s.coins < price && !cfg.unlimitedMoneyTest`, `s.coins -= price`; weaponAmmo/armorSlot/buildingSlot (purchase.go:23-45); ammoStack=30 (purchase.go:103)
  - symbols: handleCSPurchase (purchase.go:141); weaponAmmo; armorSlot; buildingSlot; ammoStack
  - cmds: 408
  - files: match-server/purchase.go
  - confidence: high
- **Player HP is const maxHP=200 (starting and max). initHP seeds every player and the bot to maxHP; entityHP falls back to maxHP. No per-match HP field.**
  - evidence: player.go:25 `maxHP = 200 // starting/max HP`; combat.go:25-30 initHP sets m.hp[p.entityID]=maxHP and bot=maxHP; combat.go:41 entityHP default maxHP; priPayload cssync.go:42 CurHP/MaxHP: maxHP
  - symbols: maxHP (player.go:25); initHP (combat.go); entityHP (combat.go:41)
  - cmds: 900
  - files: match-server/player.go; match-server/combat.go; match-server/cssync.go
  - confidence: high
- **Starting loadout is hardcoded in session.giveLoadout: fist (SlotMelee), USP (uspData=3) in SlotSecondary with pistolAmmo=202, 2x medkits (medkitData=102, count 2) + maxed USP attachments; cfg.infiniteGloo seeds 5 gloo walls (glooDataID=1201). reissueLoadout refills mags + resets medkits to 2 for survivors.**
  - evidence: cssync.go:97-166 giveLoadout: `const uspData = 3; const pistolAmmo = 202`; fist+USP equipment; clientUIDs medkit count:2; infiniteGloo appends glooDataID 5x (cssync.go:145-150); medkitData=102 (medkit.go:21), glooDataID=1201 (gloo.go:34); reissueLoadout (cssync.go:185)
  - symbols: giveLoadout (cssync.go:97); uspData=3; pistolAmmo=202; medkitData=102 (medkit.go:21); glooDataID=1201 (gloo.go:34)
  - cmds: 174; 327
  - files: match-server/cssync.go; match-server/medkit.go; match-server/gloo.go
  - confidence: high
- **Mushroom/EP economy is const-driven: epPerMushroom=100, maxEP=200, mushroomsPerRound=2, epInterval=250ms (4 HP/s). Mushroom (itemMushroom=709) is an instant EP grant (no inventory item), capped per round.**
  - evidence: ep.go:13-18 `epPerMushroom=100; maxEP=200; mushroomsPerRound=2; epInterval=250*time.Millisecond`; itemMushroom=709 (purchase.go:98); grantMushroom (ep.go:23); round cap checked purchase.go:159
  - symbols: epPerMushroom; maxEP; mushroomsPerRound; itemMushroom=709
  - cmds: 408; 533
  - files: match-server/ep.go; match-server/purchase.go
  - confidence: high
- **The ONLY existing per-match config plumbing is the process-global cfg (type config, var cfg), populated once by loadConfig() from MATCH_* env at startup — per-PROCESS, NOT per-room. Knobs already wired: unlimitedMoneyTest(MATCH_UNLIMITED_MONEY), infiniteGloo(MATCH_INFINITE_GLOO), noZone, zoneStatic, matchBot(MATCH_BOT), holdPrepare, spawn, buyPhase(10s). A per-room override CANNOT reuse this (one cfg per instance); it must become a per-Match/per-session field.**
  - evidence: config.go:16-42 struct config; config.go:44 `var cfg config`; config.go:47-74 loadConfig reads os.Getenv MATCH_*; buyPhase default 10s (config.go:18,50); consumed cssync.go:26, purchase.go:165/172, match.go:357, cssync.go:145, match.go:688
  - symbols: config struct (config.go:16); cfg (config.go:44); loadConfig (config.go:47); buyPhase; unlimitedMoneyTest; infiniteGloo; matchBot; holdPrepare
  - files: match-server/config.go
  - confidence: high
- **The per-match data channel matchmaker->match-server TODAY is the prepare_token JWT, verified in token.Verify and decoded to token.Claims: aid,name,region,role,mid(MatchID),show{avatar,color,head,banner,clothes,slots,emotes,battle_flag},iat,exp. It carries ONLY per-player identity + cosmetics — NO room/match settings field.**
  - evidence: jwt.go:38-47 Claims{AccountID`aid`,Name,Region,Role,MatchID`mid`,Show,IssuedAt,Expiry}; jwt.go:24-35 Show{Avatar,Color,Head,Banner,Clothes,Slots,Emotes,BattleFlag}; Verify (jwt.go:56)
  - symbols: token.Claims (jwt.go:38); token.Show (jwt.go:24); token.Verify (jwt.go:56)
  - cmds: 440
  - files: match-server/token/jwt.go
  - confidence: high
- **At cmd 101 join, resolvePlayer(tok) verifies the token and copies Claims -> message.PlayerInfo (aliased joinPlayer): AccountID,MatchID,Role,Name + show cosmetics; invalid token falls back to a stub. PlayerInfo is the parsed per-player struct threaded into the Match — where an equivalent settings struct would arrive, but it carries no settings today.**
  - evidence: player.go:83-116 resolvePlayer: token.Verify(tok, cfg.jwtSecret); p.MatchID=claims.MatchID; p.Avatar...=s.Avatar...; playerjoin.go:20-51 PlayerInfo fields (AccountID,EntityID,Name,Role,MatchID,Avatar,Clothes,Slots,Emotes,BattleFlag,TeamIdx,SpawnPos,SpawnFace)
  - symbols: resolvePlayer (player.go:83); joinPlayer=message.PlayerInfo (player.go:78); PlayerInfo (playerjoin.go:20)
  - cmds: 101; 440
  - files: match-server/player.go; match-server/message/playerjoin.go
  - confidence: high
- **The matchmaker mints the token in buildPrepareToken(account, matchId) with fixed claims {aid,name,region,role,mid,show{...}} — the exact JS site a settings blob would be added to (and mirrored into token.Claims). match_mode is force-set to 6 in SussNtf so the client renders the CS waiting/ranking HUD.**
  - evidence: src/tcp/matchmaker.js:70-94 buildPrepareToken claims{aid,name,region,role,mid,show{...}}; jwt.sign(claims, config.match.jwtSecret); matchmaker.js:154-166 SussNtf {match_id,server_addr,secret,prepare_token,map_id,game_mode,match_mode:6,difficulty}
  - symbols: buildPrepareToken (matchmaker.js:70); formMatch (matchmaker.js:137)
  - files: src/tcp/matchmaker.js
  - confidence: high
- **Round config is streamed to the client each tick via GRI (cmd 901): CSGRIInit(maxRound, m.round-1) sets field 1 (CSFieldMaxRound) and field 2 (CSFieldCurrentRound); match-point (field 4) set when a team is one win from roundsToWin. Overriding maxRound/roundsToWin auto re-renders the client round bar/decider flag — no client wiring needed.**
  - evidence: match.go:431 `message.CSGRIInit(maxRound, uint8(m.round-1))`; match.go:435-438 `if m.teamScore[0]==roundsToWin-1 || m.teamScore[1]==roundsToWin-1 { point = 1 }` -> CSGRIMatchPoint; replication.go:77-78 CSFieldMaxRound=1/CSFieldCurrentRound=2
  - symbols: CSGRIInit (replication.go:125); CSFieldMaxRound=1; CSFieldCurrentRound=2; CSFieldMatchPoint=4
  - cmds: 901
  - files: match-server/match.go; match-server/message/replication.go
  - confidence: high
- **DISTINCT from the in-match buy economy: durable settlement rewards (credited to the lobby account via the bus) use their OWN constants in results.go — settleCoinPerKill=50, settleWinCoins=200, settleXPPerKill=10, settleWinXP=100, settleLoseXP=40 — computed in Match.publishResult. Do NOT affect in-match p.coins/buying.**
  - evidence: results.go:16-22 const block; results.go:35-51 publishResult: coins=kills*settleCoinPerKill(+settleWinCoins if win), xp=kills*settleXPPerKill+settleLoseXP/settleWinXP; publishes pb.MatchResult/pb.MatchEnded (EndType 4)
  - symbols: settleCoinPerKill; settleWinCoins; settleXPPerKill; settleWinXP; settleLoseXP; publishResult (results.go:28)
  - files: match-server/results.go
  - confidence: high
- **Roster cap and phase timings are also consts a custom room might touch: maxPlayers=8 (gates canAdmit), buyPhase default 10s, waitPlayersDur=30s, cancelHoldDur=10s.**
  - evidence: manager.go:9 `const maxPlayers = 8`; matchhandle.go:39 canAdmit `m.reserved < maxPlayers`; config.go:18/50 buyPhase 10s; match.go:107 waitPlayersDur=30s; match.go:108 cancelHoldDur=10s
  - symbols: maxPlayers (manager.go:9); canAdmit (matchhandle.go:39); buyPhase; waitPlayersDur; cancelHoldDur
  - files: match-server/manager.go; match-server/matchhandle.go; match-server/config.go; match-server/match.go
  - confidence: high

**Our hooks:**
- match-server/cssync.go:13 — const maxRound = 7: ROUND COUNT knob; streamed via CSGRIInit (match.go:431) so overriding re-renders the client round bar automatically.
- match-server/cssync.go:14 — const roundsToWin = (maxRound+1)/2: ROUNDS-TO-WIN knob; used in match-end gate (match.go:762) and match-point flag (match.go:435).
- match-server/match.go:762 — the match-end decision `if m.teamScore[0] >= roundsToWin || m.teamScore[1] >= roundsToWin || m.round >= maxRound` in Match.endFight: single place to consult per-match round limits.
- match-server/match.go:752-755 — coin-award literals in endFight: 500*roundKills (per-kill), 500*(round+1) (per-round), +500 (win). UNNAMED literals, NO per-loss bonus — a custom economy replaces all three 500s here (writes p.coins/p.award).
- match-server/purchase.go:15 — const startingCoins = 500: starting buy money; seeded in run() (match.go:356/360) and giveLoadout (cssync.go:104).
- match-server/shop.go:17 — var csShopItems []message.ShopItem (+ shop.go:7 const csShopTitle): entire shop catalogue + per-item Price/Filter/Limitation; read by both sendCSShop (cmd 407, cssync.go:229) and buy validation shopItemByID (shop.go:45).
- match-server/player.go:25 — const maxHP = 200: player start/max HP; read by initHP (combat.go:25), entityHP (combat.go:41), priPayload (cssync.go:42).
- match-server/cssync.go:97 giveLoadout — hardcoded starting loadout (const uspData=3, const pistolAmmo=202, medkit count 2; cfg.infiniteGloo seeds glooDataID=1201): override target for a starting-weapon/loadout setting.
- match-server/ep.go:13-18 — epPerMushroom=100, maxEP=200, mushroomsPerRound=2, epInterval: mushroom/EP heal economy knobs.
- match-server/config.go:16 type config + config.go:44 var cfg + loadConfig (config.go:47): EXISTING override pattern (env->global), but PROCESS-GLOBAL — cannot hold per-room settings; use only for server-wide defaults. Style-mirror knobs: unlimitedMoneyTest, infiniteGloo, noZone, zoneStatic, matchBot, holdPrepare, buyPhase.
- match-server/token/jwt.go:38 token.Claims (+ jwt.go:24 token.Show): the per-match wire channel; a settings blob is a NEW claim field here, verified in token.Verify (jwt.go:56).
- src/tcp/matchmaker.js:70 buildPrepareToken — JS producer: add the settings object to `claims` here (same config.match.jwtSecret the Go verifier uses).
- match-server/player.go:83 resolvePlayer + match-server/message/playerjoin.go:20 message.PlayerInfo (joinPlayer): where Claims copy onto the per-player struct at cmd 101 join; settings would be copied here then threaded onto new *Match fields (match.go:18, set in admitFirst/addPlayer) read by endFight/giveLoadout/sendCSShop instead of the consts.
- match-server/results.go:16 — settlement constants (settleCoinPerKill=50, settleWinCoins=200, settleXPPerKill=10, settleWinXP=100, settleLoseXP=40) in publishResult: separate lobby-reward economy (does not affect in-match buying).
- match-server/manager.go:9 const maxPlayers = 8 (gates canAdmit, matchhandle.go:39) + phase durations (buyPhase config.go:18, waitPlayersDur/cancelHoldDur match.go:107-108): secondary knobs (team size, buy-timer length).

**Open questions:**
- To make round count / coins / HP per-match, a new end-to-end path is required: token.Claims field (jwt.go) <- matchmaker buildPrepareToken (matchmaker.js) -> resolvePlayer/PlayerInfo (player.go/playerjoin.go) -> new fields on *Match (match.go) read by endFight/giveLoadout/sendCSShop. None of that exists today; every value is a const or bare literal.
- The coin bonuses at match.go:752-755 have NO named constants (all 500) and the loadout ids in giveLoadout are inline function-local consts, so a settings override first needs the literals extracted into fields/vars.
- Because all players in a match must agree on settings but each carries their OWN prepare_token, the first player's token (admitFirst) would be authoritative for match-wide settings — confirm the matchmaker writes identical settings into every player's token, or carry settings out-of-band (e.g. a Redis key keyed by mid) rather than per-player JWT.

