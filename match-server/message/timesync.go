package message

// SyncServerTime builds the cmd 1000 (S2C_UDP_SyncServerTime) payload: a single uint32
// ServerGameTickCount. The client derives its match clock as CurrentServerTime =
// ServerGameTickCount / 30 seconds (the server ticks at 30 Hz), and every CS countdown
// is (deadline - CurrentServerTime). So streaming this with tick = elapsedSeconds*30
// on the same clock our SafeZone EndTime / GRI phase param use makes those timers
// actually count down. Sent unreliably (send_option=0). See [[bot-networkaipawn]].
func SyncServerTime(tick uint32) []byte {
	w := &Writer{}
	w.U32(tick)
	return w.B
}
