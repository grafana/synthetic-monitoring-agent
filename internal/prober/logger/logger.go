package logger

type Logger interface {
	Log(keyvals ...interface{}) error
}
