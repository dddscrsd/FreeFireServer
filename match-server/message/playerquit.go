package message

// the player quit that gets broadcasted
func PlayerAlternateQuit(entity uint32) []byte {
	w := &Writer{}
	w.U32(entity)
	return w.B
}
