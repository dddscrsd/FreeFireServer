package message

// take damage packet
func TakeDamage(victim, attacker, weapon uint32, damage uint16, bodyPart int8) []byte {
	w := &Writer{}
	w.U32(attacker)
	w.U32(victim)
	w.U16(damage)
	w.I32(int32(weapon))
	w.I32(0)
	w.I8(int8(bodyPart))
	return w.B
}
