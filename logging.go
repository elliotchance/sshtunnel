package sshtunnel

type logger interface {
	Printf(string, ...interface{})
}

type logProvider func() logger

type logWrapper struct {
	logProvider logProvider
}

func (lw *logWrapper) Printf(format string, args ...interface{}) {
	l := lw.logProvider()
	if l != nil {
		l.Printf(format, args...)
	}
}
