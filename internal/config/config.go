package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Constantes do protocolo
const (
	ProtocolVersion = 1
	ChunkSize       = 1024 // bytes por segmento de dados (evitar fragmentação com MTU típica)

	// Rede / MTU
	MTUDefault        = 1500
	IPHeaderOverhead  = 20
	UDPHeaderOverhead = 8

	// Buffers de socket
	DefaultReadBuffer  = 4 << 20 // 4 MiB
	DefaultWriteBuffer = 4 << 20 // 4 MiB
)

// Constantes para mensagens de erro
const (
	ErrEmptyField     = "não pode estar vazio"
	ErrMustBePositive = "deve ser maior que zero"
)

var (
	// Timeouts e Retries
	DefaultTimeout = 2 * time.Second
	DefaultRetries = 5

	// Parâmetros de teste (simulação de perda)
	DefaultDropRate = 0.0
)

// representa um erro de configuração
type ConfigError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e ConfigError) Error() string {
	return fmt.Sprintf("config error in field '%s': %s (value: %v)", e.Field, e.Message, e.Value)
}

// representa um erro de validação
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s", e.Field, e.Message)
}

// representa as configurações do cliente GUI
type ClientSettings struct {
	Host         string  `json:"host"`
	Port         string  `json:"port"`
	LastFile     string  `json:"last_file"`
	OutputPath   string  `json:"output_path"`
	DropRate     float64 `json:"drop_rate"`
	Timeout      string  `json:"timeout"`
	Retries      int     `json:"retries"`
	WindowWidth  int     `json:"window_width"`
	WindowHeight int     `json:"window_height"`
}

// representa as configurações do servidor GUI
type ServerSettings struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	BaseDir      string `json:"base_dir"`
	WindowWidth  int    `json:"window_width"`
	WindowHeight int    `json:"window_height"`
}

// representa a configuração do cliente
type ClientConfig struct {
	Host       string        `json:"host"`
	Port       int           `json:"port"`
	FilePath   string        `json:"file_path"`
	OutputPath string        `json:"output_path"`
	DropRate   float64       `json:"drop_rate"`
	Timeout    time.Duration `json:"timeout"`
	Retries    int           `json:"retries"`
	ChunkSize  int           `json:"chunk_size"`
	BufferSize int           `json:"buffer_size"`
}

// representa a configuração do servidor
type ServerConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	BaseDir    string `json:"base_dir"`
	ChunkSize  int    `json:"chunk_size"`
	BufferSize int    `json:"buffer_size"`
	MaxClients int    `json:"max_clients"`
	LogLevel   string `json:"log_level"`
}

// representa os parâmetros da UI do cliente
type ClientUIParams struct {
	Host       string
	Port       string
	LastFile   string
	OutputPath string
	Timeout    string
	DropRate   float64
	Retries    int
}

// representa os parâmetros para validação
type ValidationParams struct {
	Host     string
	Port     string
	FilePath string
	DropRate string
	Timeout  string
	Retries  string
}

// retorna configurações padrão para o cliente
func DefaultClientSettings() *ClientSettings {
	return &ClientSettings{
		Host:         "127.0.0.1",
		Port:         "19000",
		LastFile:     "test.bin",
		OutputPath:   "",
		DropRate:     0.0,
		Timeout:      "2s",
		Retries:      5,
		WindowWidth:  700,
		WindowHeight: 600,
	}
}

// retorna configurações padrão para o servidor
func DefaultServerSettings() *ServerSettings {
	return &ServerSettings{
		Host:         "127.0.0.1",
		Port:         "19000",
		BaseDir:      ".",
		WindowWidth:  640,
		WindowHeight: 480,
	}
}

// retorna o caminho do arquivo de configuração
func getConfigPath(filename string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".udp-client")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(configDir, filename), nil
}

// carrega as configurações do cliente do arquivo
func LoadClientSettings() (*ClientSettings, error) {
	configPath, err := getConfigPath("client.json")
	if err != nil {
		return nil, err
	}

	// Se o arquivo não existe, retorna configurações padrão
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultClientSettings(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var settings ClientSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		// Se houver erro na deserialização, retorna configurações padrão
		return DefaultClientSettings(), nil
	}

	return &settings, nil
}

// salva as configurações do cliente no arquivo
func SaveClientSettings(settings *ClientSettings) error {
	configPath, err := getConfigPath("client.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// carrega as configurações do servidor do arquivo
func LoadServerSettings() (*ServerSettings, error) {
	configPath, err := getConfigPath("server.json")
	if err != nil {
		return nil, err
	}

	// Se o arquivo não existe, retorna configurações padrão
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultServerSettings(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var settings ServerSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		// Se houver erro na deserialização, retorna configurações padrão
		return DefaultServerSettings(), nil
	}

	return &settings, nil
}

// salva as configurações do servidor no arquivo
func SaveServerSettings(settings *ServerSettings) error {
	configPath, err := getConfigPath("server.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// atualiza as configurações com valores da UI
func UpdateClientSettingsFromUI(settings *ClientSettings, params ClientUIParams) {
	settings.Host = params.Host
	settings.Port = params.Port
	settings.LastFile = params.LastFile
	settings.OutputPath = params.OutputPath
	settings.DropRate = params.DropRate
	settings.Timeout = params.Timeout
	settings.Retries = params.Retries
}

// atualiza as configurações com valores da UI
func UpdateServerSettingsFromUI(settings *ServerSettings, host, port, baseDir string) {
	settings.Host = host
	settings.Port = port
	settings.BaseDir = baseDir
}

// Validação de campos

// valida um endereço de host
func ValidateHost(host string) error {
	if strings.TrimSpace(host) == "" {
		return ValidationError{Field: "host", Message: "host não pode estar vazio"}
	}

	// Verifica se é um IP válido
	if net.ParseIP(host) != nil {
		return nil
	}

	// Verifica se é um nome de host válido
	if isValidHostname(host) {
		return nil
	}

	return ValidationError{Field: "host", Message: "host inválido"}
}

// valida uma porta
func ValidatePort(port string) error {
	if strings.TrimSpace(port) == "" {
		return ValidationError{Field: "port", Message: "porta não pode estar vazia"}
	}

	p, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return ValidationError{Field: "port", Message: "porta deve ser um número"}
	}

	if p < 1 || p > 65535 {
		return ValidationError{Field: "port", Message: "porta deve estar entre 1 e 65535"}
	}

	if p < 1024 {
		return ValidationError{Field: "port", Message: "porta abaixo de 1024 pode exigir privilégios de administrador"}
	}

	return nil
}

// valida um caminho de arquivo
func ValidateFilePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return ValidationError{Field: "file_path", Message: "caminho do arquivo não pode estar vazio"}
	}

	// Verifica caracteres perigosos
	dangerousChars := []string{"..", "~", "$", "`", "|", "&", ";", "(", ")"}
	for _, char := range dangerousChars {
		if strings.Contains(path, char) {
			return ValidationError{Field: "file_path", Message: fmt.Sprintf("caminho contém caractere perigoso: %s", char)}
		}
	}

	return nil
}

// valida a taxa de descarte
func ValidateDropRate(rate string) error {
	if strings.TrimSpace(rate) == "" {
		return nil // Opcional
	}

	r, err := strconv.ParseFloat(strings.TrimSpace(rate), 64)
	if err != nil {
		return ValidationError{Field: "drop_rate", Message: "taxa de descarte deve ser um número"}
	}

	if r < 0.0 || r > 1.0 {
		return ValidationError{Field: "drop_rate", Message: "taxa de descarte deve estar entre 0.0 e 1.0"}
	}

	return nil
}


// valida o timeout
func ValidateTimeout(timeout string) error {
	if strings.TrimSpace(timeout) == "" {
		return ValidationError{Field: "timeout", Message: "timeout não pode estar vazio"}
	}

	// Verifica se é um formato de duração válido
	_, err := parseDuration(timeout)
	if err != nil {
		return ValidationError{Field: "timeout", Message: "timeout deve ser uma duração válida (ex: 2s, 5m, 1h)"}
	}

	return nil
}

// valida o número de tentativas
func ValidateRetries(retries string) error {
	if strings.TrimSpace(retries) == "" {
		return ValidationError{Field: "retries", Message: "número de tentativas não pode estar vazio"}
	}

	r, err := strconv.Atoi(strings.TrimSpace(retries))
	if err != nil {
		return ValidationError{Field: "retries", Message: "número de tentativas deve ser um número inteiro"}
	}

	if r < 0 || r > 100 {
		return ValidationError{Field: "retries", Message: "número de tentativas deve estar entre 0 e 100"}
	}

	return nil
}

// valida a lista de sequências para descarte

// valida todos os campos de uma configuração de cliente
func ValidateAll(params ValidationParams) []error {
	var errors []error

	if err := ValidateHost(params.Host); err != nil {
		errors = append(errors, err)
	}

	if err := ValidatePort(params.Port); err != nil {
		errors = append(errors, err)
	}

	if err := ValidateFilePath(params.FilePath); err != nil {
		errors = append(errors, err)
	}

	if err := ValidateDropRate(params.DropRate); err != nil {
		errors = append(errors, err)
	}


	if err := ValidateTimeout(params.Timeout); err != nil {
		errors = append(errors, err)
	}

	if err := ValidateRetries(params.Retries); err != nil {
		errors = append(errors, err)
	}


	return errors
}

// Helper functions

// verifica se é um nome de host válido
func isValidHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	// Regex para nome de host válido
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	return hostnameRegex.MatchString(hostname)
}

// tenta fazer parse de uma duração
func parseDuration(s string) (interface{}, error) {
	// Remove espaços
	s = strings.TrimSpace(s)

	// Verifica se termina com uma unidade válida
	validUnits := []string{"ns", "us", "ms", "s", "m", "h"}
	for _, unit := range validUnits {
		if strings.HasSuffix(s, unit) {
			// Remove a unidade e tenta converter o número
			numStr := strings.TrimSuffix(s, unit)
			if numStr == "" {
				return nil, fmt.Errorf("número vazio")
			}
			_, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return nil, err
			}
			return s, nil
		}
	}

	// Se não tem unidade, assume segundos
	_, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}

	return s + "s", nil
}
