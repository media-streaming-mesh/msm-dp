package util

import (
	"encoding/json"
	"log"
	"os"
)

// LogLevel represents the log levels
type LogLevel int

const (
	Trace LogLevel = iota
	Debug
	Info
	Warning
	Error
	Fatal
)

// LogType represents the log types
type LogType int

var logLevel LogLevel

const (
	Text LogType = iota
	JSON
)

// LoggerConfig represents the logging configuration
type LoggerConfig struct {
	LogLevel LogLevel
	LogType  LogType
}

// SetLogLevel sets the log level based on the provided value
func SetLogLevel(level LogLevel) {
	switch level {
	case Trace:
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		log.SetPrefix("[TRACE] ")
	case Debug:
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		log.SetPrefix("[DEBUG] ")
	case Info:
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags)
		log.SetPrefix("[INFO] ")
	case Warning:
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags)
		log.SetPrefix("[WARNING] ")
	case Error:
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		log.SetPrefix("[ERROR] ")
	case Fatal:
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		log.SetPrefix("[FATAL] ")
	}
}

// SetLogType sets the log type based on the provided value
func SetLogType(logType LogType) {
	switch logType {
	case Text:
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags)
	case JSON:
		log.SetOutput(os.Stdout)
		log.SetFlags(0)
		log.SetOutput(&jsonLogWriter{})
	}
}

// InitLogger initializes the logger based on the logger configuration provided
func InitLogger(config LoggerConfig) {
	SetLogLevel(config.LogLevel)
	SetLogType(config.LogType)
}

// GetLogTypeFromEnv retrieves the log type from the environment variable "LOG_TYPE"
func GetLogTypeFromEnv() LogType {
	logTypeStr := os.Getenv("LOG_TYPE")

	switch logTypeStr {
	case "json":
		return JSON
	default:
		return Text // Default log type
	}
}

// GetLogLevelFromEnv retrieves the log level from the environment variable "LOG_LEVEL"
func GetLogLevelFromEnv() LogLevel {
	levelStr := os.Getenv("LOG_LEVEL")

	switch levelStr {
	case "0":
		return Trace
	case "1":
		return Debug
	case "2":
		return Info
	case "3":
		return Warning
	case "4":
		return Error
	case "5":
		return Fatal
	default:
		return Info // Default log level
	}
}

// jsonLogWriter is a custom log writer for JSON logs
type jsonLogWriter struct{}

// Write writes the log message in JSON format
func (w *jsonLogWriter) Write(p []byte) (n int, err error) {
	// TO DO: Customize the JSON log format according to our(msm) needs
	// This is a simple example where each log message is an object with a "message" field
	logEntry := map[string]string{
		"message": string(p),
	}
	jsonBytes, _ := json.Marshal(logEntry)
	return os.Stdout.Write(jsonBytes)
}

// Fatalf logs a formatted message and exits the program
func Fatalf(format string, v ...interface{}) {
	msg := formatLogMessage(format, v...)
	log.Fatal(msg)
}

// Tracef prints a formatted trace log message
func Tracef(format string, v ...interface{}) {
	log.Printf("[TRACE] "+format, v...)
}

// Debugf prints a formatted debug log message
func Debugf(format string, v ...interface{}) {
	log.Printf("[DEBUG] "+format, v...)
}

// Infof prints a formatted info log message
func Infof(format string, v ...interface{}) {
	log.Printf("[INFO] "+format, v...)
}

// Warningf prints a formatted warning log message
func Warningf(format string, v ...interface{}) {
	log.Printf("[WARNING] "+format, v...)
}

// Errorf prints a formatted error log message
func Errorf(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}

// Log prints a log message
func Log(v ...interface{}) {
	log.Print(v...)
}

// Logf prints a formatted log message
func Logf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// formatLogMessage formats the log message with the provided format and arguments
func formatLogMessage(format string, v ...interface{}) string {
	return logMessagePrefix() + logMessageFormat(format, v...)
}

// logMessagePrefix returns the log message prefix based on the current log level
func logMessagePrefix() string {
	switch logLevel {
	case Trace:
		return "[TRACE] "
	case Debug:
		return "[DEBUG] "
	case Info:
		return "[INFO] "
	case Warning:
		return "[WARNING] "
	case Error:
		return "[ERROR] "
	case Fatal:
		return "[FATAL] "
	default:
		return ""
	}
}

// logMessageFormat formats the log message using the provided format and arguments
func logMessageFormat(format string, v ...interface{}) string {
	return format
}
