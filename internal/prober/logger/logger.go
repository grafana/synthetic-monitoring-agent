package logger

type Logger interface {
	Log(keyvals ...any) error
}
