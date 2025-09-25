package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// coleta métricas detalhadas de uma transferência
type TransferMetrics struct {
	// Contadores básicos
	BytesSent        uint64 `json:"bytes_sent"`
	BytesReceived    uint64 `json:"bytes_received"`
	SegmentsSent     uint64 `json:"segments_sent"`
	SegmentsReceived uint64 `json:"segments_received"`

	// Contadores de erro
	Errors          uint64 `json:"errors"`
	Timeouts        uint64 `json:"timeouts"`
	Retransmissions uint64 `json:"retransmissions"`
	NacksReceived   uint64 `json:"nacks_received"`

	// Métricas de tempo
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`

	// Métricas de performance
	AverageSpeed float64 `json:"average_speed"` // bytes/segundo
	PeakSpeed    float64 `json:"peak_speed"`    // bytes/segundo
	Efficiency   float64 `json:"efficiency"`    // (bytes úteis / bytes totais) * 100

	// Métricas de rede
	PacketLoss float64       `json:"packet_loss"` // percentual
	Latency    time.Duration `json:"latency"`     // latência média

	// Histórico de velocidades para gráficos
	SpeedHistory []SpeedPoint `json:"speed_history"`

	// Mutex para proteção
	mu sync.RWMutex
}

// representa um ponto no histórico de velocidade
type SpeedPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Speed     float64   `json:"speed"` // bytes/segundo
}

// cria uma nova instância de métricas
func NewTransferMetrics() *TransferMetrics {
	return &TransferMetrics{
		StartTime:    time.Now(),
		SpeedHistory: make([]SpeedPoint, 0),
	}
}

// adiciona bytes enviados
func (m *TransferMetrics) AddBytesSent(bytes uint64) {
	atomic.AddUint64(&m.BytesSent, bytes)
}

// adiciona bytes recebidos
func (m *TransferMetrics) AddBytesReceived(bytes uint64) {
	atomic.AddUint64(&m.BytesReceived, bytes)
}

// adiciona segmentos enviados
func (m *TransferMetrics) AddSegmentsSent(segments uint64) {
	atomic.AddUint64(&m.SegmentsSent, segments)
}

// adiciona segmentos recebidos
func (m *TransferMetrics) AddSegmentsReceived(segments uint64) {
	atomic.AddUint64(&m.SegmentsReceived, segments)
}

// adiciona um erro
func (m *TransferMetrics) AddError() {
	atomic.AddUint64(&m.Errors, 1)
}

// adiciona um timeout
func (m *TransferMetrics) AddTimeout() {
	atomic.AddUint64(&m.Timeouts, 1)
}

// adiciona uma retransmissão
func (m *TransferMetrics) AddRetransmission() {
	atomic.AddUint64(&m.Retransmissions, 1)
}

// adiciona um NACK recebido
func (m *TransferMetrics) AddNack() {
	atomic.AddUint64(&m.NacksReceived, 1)
}

// registra a velocidade atual
func (m *TransferMetrics) RecordSpeed(speed float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	point := SpeedPoint{
		Timestamp: time.Now(),
		Speed:     speed,
	}

	m.SpeedHistory = append(m.SpeedHistory, point)

	// Mantém apenas os últimos 1000 pontos para evitar uso excessivo de memória
	if len(m.SpeedHistory) > 1000 {
		m.SpeedHistory = m.SpeedHistory[len(m.SpeedHistory)-1000:]
	}

	// Atualiza velocidade de pico
	if speed > m.PeakSpeed {
		m.PeakSpeed = speed
	}
}

// finaliza as métricas e calcula valores finais
func (m *TransferMetrics) Finish() {
	m.EndTime = time.Now()
	m.Duration = m.EndTime.Sub(m.StartTime)

	if m.Duration > 0 {
		bytesReceived := atomic.LoadUint64(&m.BytesReceived)
		m.AverageSpeed = float64(bytesReceived) / m.Duration.Seconds()
	}

	// Calcula eficiência
	totalBytes := atomic.LoadUint64(&m.BytesSent) + atomic.LoadUint64(&m.BytesReceived)
	if totalBytes > 0 {
		usefulBytes := atomic.LoadUint64(&m.BytesReceived)
		m.Efficiency = (float64(usefulBytes) / float64(totalBytes)) * 100
	}

	// Calcula perda de pacotes
	segmentsSent := atomic.LoadUint64(&m.SegmentsSent)
	segmentsReceived := atomic.LoadUint64(&m.SegmentsReceived)
	if segmentsSent > 0 {
		m.PacketLoss = (float64(segmentsSent-segmentsReceived) / float64(segmentsSent)) * 100
	}
}

// retorna uma cópia das métricas atuais
func (m *TransferMetrics) GetSnapshot() TransferMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return TransferMetrics{
		BytesSent:        atomic.LoadUint64(&m.BytesSent),
		BytesReceived:    atomic.LoadUint64(&m.BytesReceived),
		SegmentsSent:     atomic.LoadUint64(&m.SegmentsSent),
		SegmentsReceived: atomic.LoadUint64(&m.SegmentsReceived),
		Errors:           atomic.LoadUint64(&m.Errors),
		Timeouts:         atomic.LoadUint64(&m.Timeouts),
		Retransmissions:  atomic.LoadUint64(&m.Retransmissions),
		NacksReceived:    atomic.LoadUint64(&m.NacksReceived),
		StartTime:        m.StartTime,
		EndTime:          m.EndTime,
		Duration:         m.Duration,
		AverageSpeed:     m.AverageSpeed,
		PeakSpeed:        m.PeakSpeed,
		Efficiency:       m.Efficiency,
		PacketLoss:       m.PacketLoss,
		Latency:          m.Latency,
		SpeedHistory:     append([]SpeedPoint(nil), m.SpeedHistory...),
	}
}

// coleta métricas do servidor
type ServerMetrics struct {
	// Contadores básicos
	TotalConnections  uint64 `json:"total_connections"`
	ActiveConnections int64  `json:"active_connections"`
	TotalBytesSent    uint64 `json:"total_bytes_sent"`
	TotalSegmentsSent uint64 `json:"total_segments_sent"`

	// Contadores de erro
	TotalErrors          uint64 `json:"total_errors"`
	TotalTimeouts        uint64 `json:"total_timeouts"`
	TotalRetransmissions uint64 `json:"total_retransmissions"`
	TotalNacksReceived   uint64 `json:"total_nacks_received"`

	// Métricas de tempo
	Uptime    time.Duration `json:"uptime"`
	StartTime time.Time     `json:"start_time"`

	// Métricas de performance
	AverageConnections float64 `json:"average_connections"`
	PeakConnections    int64   `json:"peak_connections"`

	// Histórico de conexões
	ConnectionHistory []ConnectionPoint `json:"connection_history"`

	// Mutex para proteção
	mu sync.RWMutex
}

// representa um ponto no histórico de conexões
type ConnectionPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

// cria uma nova instância de métricas do servidor
func NewServerMetrics() *ServerMetrics {
	return &ServerMetrics{
		StartTime:         time.Now(),
		ConnectionHistory: make([]ConnectionPoint, 0),
	}
}

// adiciona uma nova conexão
func (m *ServerMetrics) AddConnection() {
	atomic.AddUint64(&m.TotalConnections, 1)
	active := atomic.AddInt64(&m.ActiveConnections, 1)

	// Atualiza pico de conexões
	if active > atomic.LoadInt64(&m.PeakConnections) {
		atomic.StoreInt64(&m.PeakConnections, active)
	}

	m.recordConnectionCount(active)
}

// remove uma conexão
func (m *ServerMetrics) RemoveConnection() {
	active := atomic.AddInt64(&m.ActiveConnections, -1)
	if active < 0 {
		active = 0
		atomic.StoreInt64(&m.ActiveConnections, 0)
	}

	m.recordConnectionCount(active)
}

// adiciona bytes enviados
func (m *ServerMetrics) AddBytesSent(bytes uint64) {
	atomic.AddUint64(&m.TotalBytesSent, bytes)
}

// adiciona segmentos enviados
func (m *ServerMetrics) AddSegmentsSent(segments uint64) {
	atomic.AddUint64(&m.TotalSegmentsSent, segments)
}

// adiciona um erro
func (m *ServerMetrics) AddError() {
	atomic.AddUint64(&m.TotalErrors, 1)
}

// adiciona um timeout
func (m *ServerMetrics) AddTimeout() {
	atomic.AddUint64(&m.TotalTimeouts, 1)
}

// adiciona uma retransmissão
func (m *ServerMetrics) AddRetransmission() {
	atomic.AddUint64(&m.TotalRetransmissions, 1)
}

// adiciona um NACK recebido
func (m *ServerMetrics) AddNack() {
	atomic.AddUint64(&m.TotalNacksReceived, 1)
}

// registra o número atual de conexões
func (m *ServerMetrics) recordConnectionCount(count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	point := ConnectionPoint{
		Timestamp: time.Now(),
		Count:     count,
	}

	m.ConnectionHistory = append(m.ConnectionHistory, point)

	// Mantém apenas os últimos 1000 pontos
	if len(m.ConnectionHistory) > 1000 {
		m.ConnectionHistory = m.ConnectionHistory[len(m.ConnectionHistory)-1000:]
	}
}

// retorna uma cópia das métricas atuais
func (m *ServerMetrics) GetSnapshot() ServerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ServerMetrics{
		TotalConnections:     atomic.LoadUint64(&m.TotalConnections),
		ActiveConnections:    atomic.LoadInt64(&m.ActiveConnections),
		TotalBytesSent:       atomic.LoadUint64(&m.TotalBytesSent),
		TotalSegmentsSent:    atomic.LoadUint64(&m.TotalSegmentsSent),
		TotalErrors:          atomic.LoadUint64(&m.TotalErrors),
		TotalTimeouts:        atomic.LoadUint64(&m.TotalTimeouts),
		TotalRetransmissions: atomic.LoadUint64(&m.TotalRetransmissions),
		TotalNacksReceived:   atomic.LoadUint64(&m.TotalNacksReceived),
		Uptime:               time.Since(m.StartTime),
		StartTime:            m.StartTime,
		AverageConnections:   m.calculateAverageConnections(),
		PeakConnections:      atomic.LoadInt64(&m.PeakConnections),
		ConnectionHistory:    append([]ConnectionPoint(nil), m.ConnectionHistory...),
	}
}

// calcula a média de conexões
func (m *ServerMetrics) calculateAverageConnections() float64 {
	if len(m.ConnectionHistory) == 0 {
		return 0
	}

	var sum int64
	for _, point := range m.ConnectionHistory {
		sum += point.Count
	}

	return float64(sum) / float64(len(m.ConnectionHistory))
}

// monitora performance em tempo real
type PerformanceMonitor struct {
	metrics        *TransferMetrics
	lastUpdate     time.Time
	lastBytes      uint64
	updateInterval time.Duration
}

// cria um novo monitor de performance
func NewPerformanceMonitor(metrics *TransferMetrics) *PerformanceMonitor {
	return &PerformanceMonitor{
		metrics:        metrics,
		lastUpdate:     time.Now(),
		updateInterval: 100 * time.Millisecond,
	}
}

// atualiza as métricas de performance
func (pm *PerformanceMonitor) Update() {
	now := time.Now()
	if now.Sub(pm.lastUpdate) < pm.updateInterval {
		return
	}

	currentBytes := atomic.LoadUint64(&pm.metrics.BytesReceived)
	elapsed := now.Sub(pm.lastUpdate).Seconds()

	if elapsed > 0 {
		speed := float64(currentBytes-pm.lastBytes) / elapsed
		pm.metrics.RecordSpeed(speed)
	}

	pm.lastBytes = currentBytes
	pm.lastUpdate = now
}
