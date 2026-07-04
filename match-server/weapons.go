package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	_ "embed"
)

// Weapon / skin static data, parsed once at init from the game's CSV tables.
//
// The CSVs live in weapondata/ because go:embed can only see files inside the
// module. They are verbatim copies of ../protocol/WeaponTable.csv and
// ../protocol/WeaponSkin.csv (re-copy them if the source tables change):
//
//	cp ../protocol/WeaponTable.csv weapondata/WeaponTable.csv
//	cp ../protocol/WeaponSkin.csv  weapondata/WeaponSkin.csv
//
// Both files are plain comma-delimited UTF-8 (no BOM, no quoting) with TWO
// header rows (English names, then Chinese); real data starts at row 3. Field
// values may be blank (parsed as 0), and skin clip modifiers may be NEGATIVE.

//go:embed weapondata/WeaponTable.csv
var weaponTableCSV []byte

//go:embed weapondata/WeaponSkin.csv
var weaponSkinCSV []byte

// WeaponTable.csv column indexes (0-based). Confirmed against the header row and
// the USP data row (id 3 -> ammo_clip_size 12, ammo_id 202).
const (
	wtColID       = 0  // id           (weapon id)
	wtColClipSize = 19 // ammo_clip_size (base loaded-magazine size = Runtime)
	wtColAmmoID   = 22 // ammo_id      (kept for reference; weapon->ammo map is in purchase.go)
	wtColSkinID   = 73 // id_weaponskin (default/base skin id; OFTEN BLANK -> 0)
)

// WeaponSkin.csv column indexes (0-based).
const (
	wsColSkinID   = 0 // SkinID
	wsColWeaponID = 4 // WeaponID (which weapon this skin belongs to)
	wsColClipMod  = 6 // ammo_clip_size (per-skin magazine MODIFIER; often blank; may be negative)
)

var (
	// weaponClipSize[weaponID] = base ammo_clip_size (the default Runtime value).
	weaponClipSize = map[uint32]uint32{}
	// weaponDefaultSkin[weaponID] = id_weaponskin (0 when the field is blank).
	weaponDefaultSkin = map[uint32]uint32{}
	// weaponSkins[weaponID] = every SkinID whose WeaponID == weaponID.
	weaponSkins = map[uint32][]uint32{}
	// skinClipMod[skinID] = that skin's ammo_clip_size modifier (0 if blank; MAY be negative).
	skinClipMod = map[uint32]int{}
)

func init() {
	parseWeaponTable(weaponTableCSV)
	parseWeaponSkins(weaponSkinCSV)
}

// newCSVReader returns a csv.Reader tolerant of the tables' quirks: variable
// field counts (FieldsPerRecord = -1) and any stray quotes (LazyQuotes).
func newCSVReader(b []byte) *csv.Reader {
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true
	return r
}

func parseWeaponTable(b []byte) {
	r := newCSVReader(b)
	for row := 0; ; row++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip any malformed line rather than fail the build/run
		}
		if row < 2 { // skip the 2 header rows
			continue
		}
		if len(rec) <= wtColSkinID {
			continue
		}
		id, ok := parseU32(rec[wtColID])
		if !ok {
			continue
		}
		if clip, ok := parseU32(rec[wtColClipSize]); ok {
			weaponClipSize[id] = clip
		}
		if skin, ok := parseU32(rec[wtColSkinID]); ok {
			weaponDefaultSkin[id] = skin // absent/blank -> map default 0 on lookup
		}
	}
}

func parseWeaponSkins(b []byte) {
	r := newCSVReader(b)
	for row := 0; ; row++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if row < 2 {
			continue
		}
		if len(rec) <= wsColClipMod {
			continue
		}
		skinID, ok := parseU32(rec[wsColSkinID])
		if !ok {
			continue
		}
		weaponID, ok := parseU32(rec[wsColWeaponID])
		if !ok {
			continue // a skin with no owning weapon is unusable for lookup
		}
		weaponSkins[weaponID] = append(weaponSkins[weaponID], skinID)
		if mod, ok := parseInt(rec[wsColClipMod]); ok {
			skinClipMod[skinID] = mod
		}
	}
}

// SkinForWeapon returns the skin id the player has equipped for weaponID (fix 4).
// It scans the weapon's candidate skins (WeaponSkin rows with WeaponID==weaponID);
// if any of them appears in the player's loadout Slots, that one is returned.
// Otherwise it falls back to the weapon's default skin (id_weaponskin), or 0.
// slots is PlayerInfo.Slots, i.e. []uint32 of equipped item ids.
func SkinForWeapon(weaponID uint32, slots []uint32) uint32 {
	if cands := weaponSkins[weaponID]; len(cands) > 0 && len(slots) > 0 {
		set := make(map[uint32]struct{}, len(cands))
		for _, c := range cands {
			set[c] = struct{}{}
		}
		for _, s := range slots { // player's equipped order wins
			if _, ok := set[s]; ok {
				return s
			}
		}
	}
	return weaponDefaultSkin[weaponID] // 0 if the weapon has no default skin
}

// ClipSize returns the loaded-magazine size (the InvItem.Runtime value) for a
// weapon wearing the given skin: base ammo_clip_size + the skin's modifier
// (fix 3). skinID 0 (or any unknown skin) contributes no modifier. The result
// is clamped to >= 0 because skin modifiers can be negative.
func ClipSize(weaponID, skinID uint32) uint32 {
	v := int(weaponClipSize[weaponID]) + skinClipMod[skinID]
	if v < 0 {
		v = 0
	}
	return uint32(v)
}

// parseU32 parses a trimmed decimal uint32. Blank -> (0, false).
func parseU32(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// parseInt parses a trimmed decimal signed int (skin modifiers may be negative).
// Blank -> (0, false).
func parseInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
