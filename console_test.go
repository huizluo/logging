package logging

import "testing"

func testConsoleCalls(l *Logger) {
	l.Critical("critle")
	l.Error("error")
	l.Warning("warning")
	l.Notice("notice")
	l.Info("info")
	l.Debug("debug")
}

func TestConsole(t *testing.T) {
	log1 := NewLogger(10000)
	log1.EnableDepth(true)
	log1.SetLogger("console", "")
	testConsoleCalls(log1)

	log2 := NewLogger(100)
	log2.SetLogger("console", `{"level":6}`)
	testConsoleCalls(log2)
}
