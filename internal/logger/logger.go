package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// representa os níveis de log
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// retorna a representação string do nível de log
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// retorna a cor ANSI para o nível de log
func (l LogLevel) Color() string {
	switch l {
	case DEBUG:
		return "\033[37m" // Cinza
	case INFO:
		return "\033[34m" // Azul
	case WARN:
		return "\033[33m" // Amarelo
	case ERROR:
		return "\033[31m" // Vermelho
	case FATAL:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// representa um logger estruturado
type Logger struct {
	level    LogLevel
	output   io.Writer
	prefix   string
	file     *os.File
	useColor bool
}

// cria um novo logger
func NewLogger(level LogLevel, output io.Writer, prefix string) *Logger {
	return &Logger{
		level:    level,
		output:   output,
		prefix:   prefix,
		useColor: true,
	}
}

// cria um logger que escreve em arquivo
func NewFileLogger(level LogLevel, logDir, prefix string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", prefix, time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	return &Logger{
		level:    level,
		output:   file,
		prefix:   prefix,
		file:     file,
		useColor: false,
	}, nil
}

// fecha o logger se estiver usando arquivo
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// define o nível de log
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// define se deve usar cores
func (l *Logger) SetColor(useColor bool) {
	l.useColor = useColor
}

// escreve uma mensagem de log
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	// Obtém informações do caller
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	} else {
		file = filepath.Base(file)
	}

	// Formata a mensagem
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Monta a linha de log
	var logLine string
	if l.useColor {
		logLine = fmt.Sprintf("%s[%s] %s %s:%d %s\033[0m\n",
			level.Color(),
			timestamp,
			level.String(),
			file,
			line,
			message)
	} else {
		logLine = fmt.Sprintf("[%s] %s %s:%d %s\n",
			timestamp,
			level.String(),
			file,
			line,
			message)
	}

	// Escreve no output
	if l.prefix != "" {
		logLine = fmt.Sprintf("[%s] %s", l.prefix, logLine)
	}

	l.output.Write([]byte(logLine))
}

// escreve uma mensagem de debug
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// escreve uma mensagem de informação
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// escreve uma mensagem de aviso
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// escreve uma mensagem de erro
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// escreve uma mensagem fatal e termina o programa
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
	os.Exit(1)
}

// adiciona um campo estruturado ao log
func (l *Logger) WithField(key, value string) *Logger {
	return &Logger{
		level:    l.level,
		output:   l.output,
		prefix:   fmt.Sprintf("%s %s=%s", l.prefix, key, value),
		file:     l.file,
		useColor: l.useColor,
	}
}

// adiciona múltiplos campos estruturados ao log
func (l *Logger) WithFields(fields map[string]string) *Logger {
	var fieldStrs []string
	for key, value := range fields {
		fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%s", key, value))
	}

	return &Logger{
		level:    l.level,
		output:   l.output,
		prefix:   fmt.Sprintf("%s %s", l.prefix, strings.Join(fieldStrs, " ")),
		file:     l.file,
		useColor: l.useColor,
	}
}

// Logger global para uso em todo o projeto
var (
	DefaultLogger *Logger
	ClientLogger  *Logger
	ServerLogger  *Logger
)

// inicializa os loggers globais
func InitLoggers(logDir string) error {
	// Logger padrão (stdout)
	DefaultLogger = NewLogger(INFO, os.Stdout, "")

	// Logger do cliente
	clientLogger, err := NewFileLogger(DEBUG, logDir, "client")
	if err != nil {
		return err
	}
	ClientLogger = clientLogger

	// Logger do servidor
	serverLogger, err := NewFileLogger(DEBUG, logDir, "server")
	if err != nil {
		return err
	}
	ServerLogger = serverLogger

	return nil
}

// fecha todos os loggers
func CloseLoggers() {
	if ClientLogger != nil {
		ClientLogger.Close()
	}
	if ServerLogger != nil {
		ServerLogger.Close()
	}
}

// Funções de conveniência para usar o logger padrão
func Debug(format string, args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Debug(format, args...)
	}
}

func Info(format string, args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Info(format, args...)
	}
}

func Warn(format string, args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Warn(format, args...)
	}
}

func Error(format string, args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Error(format, args...)
	}
}

func Fatal(format string, args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Fatal(format, args...)
	}
}

// configura o logger padrão do Go
func SetupDefaultLogger() {
	if DefaultLogger != nil {
		log.SetFlags(0)
		log.SetOutput(DefaultLogger.output)
	}
}
