package config

type WriteData struct {
	data string
}

func (w *WriteData) Write(p []byte) (n int, err error) {
	w.data = string(p)
	return len(p), nil
}
