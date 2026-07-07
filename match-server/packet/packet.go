package packet

import (
	"encoding/binary"
	"errors"

	"libmadoka/match-server/crypto"
)

// MsgKey is the fixed first byte of every match packet (GCommon::UDPSession
// ::HandleRecv rejects anything else).
const MsgKey = 0x6C

// send_option values (see UDP_MATCH_PROTOCOL.md).
const (
	SendUnreliable = 0 // cmd routed normally, no seq/order
	SendHello      = 1 // connection/HELLO response (S2C_Hello_Res)
	SendReliable   = 2 // normal reliable app packet
	SendKick       = 3 // "kicked by server"
	SendVar        = 4 // variables
	SendReliable5  = 5 // reliable variables - never used?
)

// flags bits.
const (
	FlagEncrypted = 1 // payload is TEA(SECRET_KEY)
	FlagGzip      = 2 // payload is gzip(SECRET_KEY)
)

// cmds (message.OPNCEEBBMJH enum values recovered from the client).
const (
	CmdHello               = 1   // client HELLO / server S2C_Hello_Res
	CmdAck                 = 2   // reliable ACK / keepalive
	CmdPing                = 3   // unreliable ping (client<->server)
	CmdReconnect           = 5   // client reconnect hello (carries SessionKey)
	CmdBattleStart         = 110 // server -> client: JAKKPBMHLAI (start the match)
	CmdForceSync           = 144 // server -> client: JIIKBLKJCKM force-sync (SyncStateWithServer) — position + phys state (handler DDBOMMOJHDD)
	CmdTeleport            = 145 // server -> client: JIIKBLKJCKM teleport (SyncTeleportInfo) — position + ROTATION (handler OHPEJIKNCMD)
	CmdJoinMatchFinished   = 130 // server -> client: LGBENPIAFIN (join complete / HUD)
	CmdJoinMatchRes        = 100 // server -> client: LGIGCGIDOKP (join result)
	CmdPlayerJoin          = 101 // server -> client: GKBDLJFGGMI (player join)
	CmdMatchEnd            = 103 // server -> client: RUDP_MATCH_END (also used as the SendKick(so=3) carrier on shutdown)
	CmdChangeHeldItem      = 108 // client -> server: RUDP_CHANGE_INVENTORY_ON_HAND — [entity u32][itemUnique u32]
	CmdPickupInventory     = 111 // client -> server: LHODJLEHDND — RUDP_PICKUP_INVENTORY (pick a ground item by UniqueID)
	CmdDropInventory       = 112 // client <-> server: KJBONEENCAL — RUDP_DROP_INVENTORY (req: u32 unique,u32 count,u8 reason; res reuses class)
	CmdUseInventory        = 113 // client -> server: RUDP_USE_INVENTORY — finished channelling a consumable ([u32 uid][u32 count][u32 param]); server applies the effect + tracks the consume
	CmdTryUseInventory     = 131 // client -> server: RUDP_TRYUSE_INVENTORY (KOMODKGGDBG, [u32 entity][u32 item][u32 tick]) — started channelling a consumable
	CmdEquipAttachment     = 122 // client -> server: RUDP_EQUIP_ATTACHMENT — client's manual attachment equip (server auto-maxes on buy, so logged/ignored)
	CmdUnequipAttachment   = 123 // client -> server: RUDP_UNEQUIP_ATTACHMENT — IGNORED (attachments are kept locked on the weapon)
	CmdAttachmentChanged   = 124 // server -> client: KBDODAHANGB — RUDP_ATTACHMENT_CHANGED (force-equip a maxed attachment onto a bought weapon)
	CmdAddPickup           = 114 // server -> client: INMIMMDMPFM — RUDP_ADD_PICKUP (one ground-loot item at a position)
	CmdDelPickup           = 115 // server -> client: KBHENKFBDAJ — RUDP_DEL_PICKUP (remove one ground-loot item)
	CmdAddIcewall          = 218 // client <-> server: RUDP_ADD_ICEWALL (0xDA) — C->S place request (JEGADMHBFKB, 29B); S->C spawn broadcast (KACGKFMOHNP, 41B)
	CmdIcewallTakeDamage   = 219 // client -> server: CHDLJFJCPFN — RUDP_ICEWALL_TAKE_DAMAGE (0xDB) (wallID + delta damage; server subtracts HP)
	CmdResyncIcewall       = 220 // server -> client: EKKLNKLGFIM — RUDP_RESYNC_ICEWALL (0xDC) ([i16 count] + JJIODFAHEOG×N; full re-add of live walls)
	CmdRemoveIcewall       = 221 // server -> client: APPOHELKIIG — RUDP_REMOVE_ICEWALL (0xDD) ([u32 wallID]; break/timeout/FIFO-evict)
	CmdAddPickupList       = 225 // server -> client: ANICDCDDCLN — RUDP_ADD_PICKUPLIST (a shared-position list of items in a container)
	CmdAddContainer        = 227 // server -> client: GONMBPDMEBE — RUDP_ADD_CONTAINER ([i16 count]{u32,f32,bool}; no position)
	CmdDelContainer        = 228 // server -> client: OPNFJLJGPFC — RUDP_DEL_CONTAINER ([u16 containerID][u8 containerType])
	CmdTakeDamage          = 106 // client -> server: hit report (victim u32, damage u16, attacker u32, ...); server applies HP
	CmdDead                = 107 // server -> client: OGCHKCGKGKN — entity died (victim, killer, position, ...)
	CmdKnockDown           = 139 // server -> client: NKDBFGLPCCF — RUDP_KNOCK_DOWN (DBNO: downed+bleeding, still alive; distinct from cmd 107)
	CmdKnockRevive         = 140 // server -> client: DJOOMNBJMOG — RUDP_KNOCK_REVIVE (teammate revive done: revivedId, rescuerId)
	CmdStartRescue         = 142 // bidirectional: client EMOBCDJEOLN START_RESCURE (u32 target, u64 startMs); server DCBAMPDIHIG rescue-ack/progress (bool ok, u32 RESCUER, u32 RESCUED, u64 start, u64 end, u32 result) — same opcode; 2nd u32 (rescued) drives the downed player's being-revived indicator
	CmdStopRescue          = 143 // client -> server: CNCBPOJGNGK — RUDP_STOP_RESCURE (u32 targetId)
	CmdBindPRI             = 118 // server -> client: S2C_RUDP_BindPRI (reliable) — maps RepIDs to entities; local player FIRST
	CmdSyncInventory       = 174 // server -> client: BGKCMKNDAGA (typed) — player inventory/equipment sync (additive; send once)
	CmdRemoveInventoryList = 327 // server -> client: ABJFDIFIILN (typed) — REMOVE items by unique from a player's inventory (the only remove path)
	CmdPlayerQuitReq       = 191
	CmdPlayerQuitRes       = 192  // server -> client: typed — player quit
	CmdCSShop              = 407  // server -> client: NDBLNLLMMJA (typed) — Contra Squad buy-phase shop (LOJA) item list
	CmdCSPurchase          = 408  // client <-> server: IIOCLGDNBBB (typed) — buy request / result (result carries new coin balance)
	CmdCSRoundResult       = 409  // server -> client: DMJPAJFMMMB (typed) — round result (winner team id -> VICTORY/DEFEAT + score tally)
	CmdSafeZoneChange      = 117  // server -> client: KGOHADAMBLI (typed) — circular safezone (outer->inner shrink; EndMs drives the Fight timer)
	CmdNotifyRevive        = 388  // server -> client: MJCMIMNHILD (typed) — revive an EXISTING dead player at a position (keeps team/faction/loadout)
	CmdPRISync             = 900  // server -> client: PRI replication (per-player), 0x384. (1.70.1; ref's 500 is an unrelated typed handler AJNCGCHMNIP)
	CmdGRISync             = 901  // server -> client: GRI replication (game-wide match state), single block, 0x385. (1.70.1; ref's 501 is an unrelated typed handler JPKINIIHOFP)
	CmdJoinMatchPrepare    = 439  // client -> server: RUDP_JOIN_MATCH_PREPARE — [u32 totalLen][token chunk] (big prepare_token, bigger half)
	CmdJoinMatchPost       = 440  // client -> server: RUDP_JOIN_MATCH_POST (match metadata + small/tail prepare_token)
	CmdSyncServerTime      = 1000 // server -> client: uint32 ServerGameTickCount; client clock = tick/30s (drives the CS timers)
	CmdClientPos           = 1001 // client -> server: per-tick position/state update (carries the local player's world pos)
	CmdClientPosV2         = 1012 // server -> client: UDP_PLAYER_STATE_SYNC_V2 — the remote-movement broadcast 1.70.1 actually applies (1001 downstream is ignored on this build)
)

var (
	ErrShort  = errors.New("packet: too short")
	ErrMsgKey = errors.New("packet: bad msg_key")
	ErrCRC    = errors.New("packet: bad crc7")
	ErrGzip   = errors.New("packet: gzip payload not supported")
)

// IsReliable mirrors GCommon::UDPMsgPacket::IsReliable.
func IsReliable(cmd uint16, sendOption byte) bool {
	if cmd == 2 {
		return false
	}
	return sendOption == 1 || sendOption == 2 || sendOption == 5
}

// Packet is a decoded/decodable match packet. Payload is always PLAINTEXT here
// (Decode decrypts; Encode encrypts when Flags&FlagEncrypted).
type Packet struct {
	SendOption byte
	Cmd        uint16
	SeqID      uint16
	OrderID    uint16
	Flags      byte
	Payload    []byte
}

// Decode parses one inbound datagram. key is the 16-byte TEA key.
func Decode(buf, key []byte) (*Packet, error) {
	if len(buf) < 8 {
		return nil, ErrShort
	}
	if buf[0] != MsgKey {
		return nil, ErrMsgKey
	}
	if crypto.CRC7(0, buf[2:]) != buf[1] {
		return nil, ErrCRC
	}
	p := &Packet{}
	off := 2
	p.SendOption = buf[off]
	off++
	p.Cmd = binary.LittleEndian.Uint16(buf[off:])
	off += 2
	if IsReliable(p.Cmd, p.SendOption) {
		if len(buf) < 12 {
			return nil, ErrShort
		}
		p.SeqID = binary.LittleEndian.Uint16(buf[off:])
		off += 2
		p.OrderID = binary.LittleEndian.Uint16(buf[off:])
		off += 2
	}
	p.Flags = buf[off]
	off++
	length := int(binary.LittleEndian.Uint16(buf[off:]))
	off += 2
	if off+length > len(buf) {
		return nil, ErrShort
	}
	payload := buf[off : off+length]
	if p.Flags&FlagGzip != 0 {
		return nil, ErrGzip
	}
	if p.Flags&FlagEncrypted != 0 {
		dec, err := crypto.TeaDecrypt(payload, key)
		if err != nil {
			return nil, err
		}
		payload = dec
	}
	p.Payload = append([]byte(nil), payload...)
	return p, nil
}

// Encode serializes the packet to wire bytes. Encrypts the payload when
// Flags&FlagEncrypted is set.
func (p *Packet) Encode(key []byte) ([]byte, error) {
	payload := p.Payload
	if p.Flags&FlagEncrypted != 0 {
		enc, err := crypto.TeaEncrypt(payload, key)
		if err != nil {
			return nil, err
		}
		payload = enc
	}
	body := make([]byte, 0, 10+len(payload))
	body = append(body, p.SendOption)
	body = binary.LittleEndian.AppendUint16(body, p.Cmd)
	if IsReliable(p.Cmd, p.SendOption) {
		body = binary.LittleEndian.AppendUint16(body, p.SeqID)
		body = binary.LittleEndian.AppendUint16(body, p.OrderID)
	}
	body = append(body, p.Flags)
	body = binary.LittleEndian.AppendUint16(body, uint16(len(payload)))
	body = append(body, payload...)

	out := make([]byte, 0, 2+len(body))
	out = append(out, MsgKey, crypto.CRC7(0, body))
	out = append(out, body...)
	return out, nil
}

// BuildAck builds the unreliable cmd=2 ACK for a received reliable seq.
func BuildAck(seq uint16, ackBits uint32, key []byte) ([]byte, error) {
	payload := make([]byte, 6)
	binary.LittleEndian.PutUint16(payload[0:], seq)
	binary.LittleEndian.PutUint32(payload[2:], ackBits)
	return (&Packet{SendOption: SendUnreliable, Cmd: CmdAck, Payload: payload}).Encode(key)
}

// BuildHelloRes builds the S2C_Hello_Res payload:
//
//	keyLen(i32) | SessionKey(UTF-8) | OrderID(u16) | RequiredID(u16) | EnableFastProto(u8)
func BuildHelloRes(sessionKey string, orderID, requiredID uint16, enableFastProto bool) []byte {
	b := make([]byte, 0, 9+len(sessionKey))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(sessionKey)))
	b = append(b, []byte(sessionKey)...)
	b = binary.LittleEndian.AppendUint16(b, orderID)
	b = binary.LittleEndian.AppendUint16(b, requiredID)
	var fp byte
	if enableFastProto {
		fp = 1
	}
	return append(b, fp)
}
