package app

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	logFile    *os.File
	tuiLogFile *os.File
	logger     *slog.Logger
	diagLogs   []string
	diagMutex  sync.Mutex

	tuiLogMu    sync.Mutex
	tuiLogUsers int
)

func addDiagLog(msg string) {
	diagMutex.Lock()
	defer diagMutex.Unlock()
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	diagLogs = append(diagLogs, msg)
	if len(diagLogs) > 300 {
		diagLogs = diagLogs[len(diagLogs)-300:]
	}
}

// GetDiagLogs returns a copy of the recent diagnostic log lines, oldest first,
// capped at the last 300; the TUI's log view reads it.
func GetDiagLogs() []string {
	diagMutex.Lock()
	defer diagMutex.Unlock()
	out := make([]string, len(diagLogs))
	copy(out, diagLogs)
	return out
}

type diagSplitWriter struct {
	file io.Writer
}

func (d *diagSplitWriter) Write(p []byte) (n int, err error) {
	n, err = d.file.Write(p)
	addDiagLog(string(p))
	return n, err
}

func openLogFile(configDir, filename string) (*os.File, error) {
	logDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}

// InitLogger opens the log file under configDir/logs, clearing previous logs
// unless logAppend is set, and installs it as the default slog output. An empty
// configDir or an unopenable file falls back to logging on standard error.
func InitLogger(configDir string, logAppend bool) {
	if configDir == "" {
		handler := slog.NewTextHandler(os.Stderr, nil)
		logger = slog.New(handler)
		slog.SetDefault(logger)
		return
	}

	logDir := filepath.Join(configDir, "logs")

	if !logAppend {
		if files, err := os.ReadDir(logDir); err == nil {
			for _, f := range files {
				if !f.IsDir() {
					_ = os.Remove(filepath.Join(logDir, f.Name()))
				}
			}
		}
	}

	var err error
	logFile, err = openLogFile(configDir, AppName+".log")
	if err != nil {
		handler := slog.NewTextHandler(os.Stderr, nil)
		logger = slog.New(handler)
		slog.SetDefault(logger)
		return
	}

	log.SetOutput(logFile)

	logLevel := getLogLevelFromArgs()
	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: logLevel,
	})
	logger = slog.New(handler)
	slog.SetDefault(logger)
	slog.Debug("Logger initialized", slog.String("level", logLevel.String()))
}

// CloseLogger flushes and closes the main log file, if one was opened.
func CloseLogger() {
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
	}
}

// InitTuiLogger installs the TUI's own log file, teeing writes into the
// in-memory buffer GetDiagLogs reads. It falls back to standard error when the
// file cannot be opened.
//
// Calls nest: each is matched by a CloseTuiLogger, and only the first opens
// the file and only the last closes it. The new-mail watcher that runs beside
// the TUI starts before the TUI does and stops after it, and logs throughout.
func InitTuiLogger(configDir string) {
	tuiLogMu.Lock()
	defer tuiLogMu.Unlock()
	tuiLogUsers++
	if tuiLogUsers > 1 {
		return
	}
	openTuiLogger(configDir)
}

// openTuiLogger opens the TUI log and makes it slog's default.
func openTuiLogger(configDir string) {
	logDir := filepath.Join(configDir, "logs")

	var err error
	tuiLogFile, err = openLogFile(configDir, "tui.log")
	if err != nil {
		if mkErr := os.MkdirAll(logDir, 0700); mkErr != nil {
			handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: getLogLevelFromArgs(),
			})
			logger = slog.New(handler)
			slog.SetDefault(logger)
			return
		}
		tuiLogFile, err = openLogFile(configDir, "tui.log")
		if err != nil {
			handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: getLogLevelFromArgs(),
			})
			logger = slog.New(handler)
			slog.SetDefault(logger)
			return
		}
	}

	splitWriter := &diagSplitWriter{file: tuiLogFile}
	log.SetOutput(splitWriter)
	logLevel := getLogLevelFromArgs()
	handler := slog.NewTextHandler(splitWriter, &slog.HandlerOptions{
		Level: logLevel,
	})
	logger = slog.New(handler)
	slog.SetDefault(logger)
	slog.Debug("TUI Logger initialized", slog.String("path", filepath.Join(logDir, "tui.log")), slog.String("level", logLevel.String()))
}

// CloseTuiLogger flushes and closes the TUI log file, if one was opened, once
// the last InitTuiLogger caller is done with it.
func CloseTuiLogger() {
	tuiLogMu.Lock()
	defer tuiLogMu.Unlock()
	if tuiLogUsers > 0 {
		tuiLogUsers--
	}
	if tuiLogUsers > 0 {
		return
	}
	if tuiLogFile != nil {
		_ = tuiLogFile.Sync()
		_ = tuiLogFile.Close()
		tuiLogFile = nil
	}
}

func getLogLevelFromArgs() slog.Level {
	verboseCount := 0
	for _, arg := range os.Args {
		switch {
		case arg == "-v" || arg == "--verbose":
			verboseCount++
		case arg == "-vv":
			verboseCount += 2
		case arg == "-vvv":
			verboseCount += 3
		}
	}

	if verboseCount == 1 {
		return slog.LevelDebug
	}
	if verboseCount > 1 {
		return slog.Level(-8)
	}
	return slog.LevelInfo
}
