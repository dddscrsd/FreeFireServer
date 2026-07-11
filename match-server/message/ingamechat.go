package message

func SendInGameMessages(message []string, author string) []byte {
	w := &Writer{}
	w.I16(int16(len(message)))
	for _, msg := range message {
		w.Str(author)
		w.Str(msg)
	}
	return w.B
}
