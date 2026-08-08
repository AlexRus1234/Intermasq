// Package version содержит единственный источник истины для версии сборки.
// Default — "1.0.0-pre"; перекрывается через -ldflags при релизной сборке.
package version

// Version приложения. Перекрывается через:
//
//	-ldflags "-X intermask/internal/version.Version=<tag>"
var Version = "1.0.0-pre"
