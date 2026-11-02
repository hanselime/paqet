package metrics

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	Role              string         `json:"role"`
	Timestamp         string         `json:"timestamp"`
	UptimeSeconds     int64          `json:"uptime_seconds"`
	BytesIn           uint64         `json:"bytes_in"`
	BytesOut          uint64         `json:"bytes_out"`
	PacketsIn         uint64         `json:"packets_in"`
	PacketsOut        uint64         `json:"packets_out"`
	BytesInPerSec     float64        `json:"bytes_in_per_sec"`
	BytesOutPerSec    float64        `json:"bytes_out_per_sec"`
	ActiveConnections int64          `json:"active_connections"`
	ActiveStreams     int64          `json:"active_streams"`
	SuccessTotal      uint64         `json:"success_total"`
	FailureTotal      uint64         `json:"failure_total"`
	SuccessTCP        uint64         `json:"success_tcp"`
	FailureTCP        uint64         `json:"failure_tcp"`
	SuccessUDP        uint64         `json:"success_udp"`
	FailureUDP        uint64         `json:"failure_udp"`
	SuccessSOCKS      uint64         `json:"success_socks"`
	FailureSOCKS      uint64         `json:"failure_socks"`
	SuccessFwdTCP     uint64         `json:"success_forward_tcp"`
	FailureFwdTCP     uint64         `json:"failure_forward_tcp"`
	SuccessFwdUDP     uint64         `json:"success_forward_udp"`
	FailureFwdUDP     uint64         `json:"failure_forward_udp"`
	Connections       []ConnSnapshot `json:"connections"`
}

type ConnSnapshot struct {
	ID             uint64  `json:"id"`
	RemoteAddr     string  `json:"remote_addr"`
	LocalAddr      string  `json:"local_addr"`
	StartedAt      string  `json:"started_at"`
	LastSeen       string  `json:"last_seen"`
	Status         string  `json:"status"`
	ClosedAt       string  `json:"closed_at"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	BytesIn        uint64  `json:"bytes_in"`
	BytesOut       uint64  `json:"bytes_out"`
	BytesInPerSec  float64 `json:"bytes_in_per_sec"`
	BytesOutPerSec float64 `json:"bytes_out_per_sec"`
	ActiveStreams  int64   `json:"active_streams"`
}

var startTime = time.Now()

var ingressBytes uint64
var egressBytes uint64
var ingressPackets uint64
var egressPackets uint64
var activeConnections int64
var activeStreams int64
var successTotal uint64
var failureTotal uint64
var successTCP uint64
var failureTCP uint64
var successUDP uint64
var failureUDP uint64
var successSOCKS uint64
var failureSOCKS uint64
var successFwdTCP uint64
var failureFwdTCP uint64
var successFwdUDP uint64
var failureFwdUDP uint64

var role atomic.Value

type connStats struct {
	id            uint64
	remoteAddr    string
	localAddr     string
	startedAt     time.Time
	lastSeen      atomic.Value
	closedAt      atomic.Value
	bytesIn       uint64
	bytesOut      uint64
	activeStreams int64
}

var connID uint64
var connMu sync.RWMutex
var conns = make(map[uint64]*connStats)

const connectionRetention = 5 * time.Minute

func SetRole(r string) {
	role.Store(r)
}

func AddIngress(bytes int, packets int) {
	if bytes > 0 {
		atomic.AddUint64(&ingressBytes, uint64(bytes))
	}
	if packets > 0 {
		atomic.AddUint64(&ingressPackets, uint64(packets))
	}
}

func AddEgress(bytes int, packets int) {
	if bytes > 0 {
		atomic.AddUint64(&egressBytes, uint64(bytes))
	}
	if packets > 0 {
		atomic.AddUint64(&egressPackets, uint64(packets))
	}
}

func IncConnections() {
	atomic.AddInt64(&activeConnections, 1)
}

func DecConnections() {
	atomic.AddInt64(&activeConnections, -1)
}

func IncStreams() {
	atomic.AddInt64(&activeStreams, 1)
}

func DecStreams() {
	atomic.AddInt64(&activeStreams, -1)
}

func IncSuccess() {
	atomic.AddUint64(&successTotal, 1)
}

func IncFailure() {
	atomic.AddUint64(&failureTotal, 1)
}

func IncSuccessTCP() {
	atomic.AddUint64(&successTCP, 1)
}

func IncFailureTCP() {
	atomic.AddUint64(&failureTCP, 1)
}

func IncSuccessUDP() {
	atomic.AddUint64(&successUDP, 1)
}

func IncFailureUDP() {
	atomic.AddUint64(&failureUDP, 1)
}

func IncSuccessSOCKS() {
	atomic.AddUint64(&successSOCKS, 1)
}

func IncFailureSOCKS() {
	atomic.AddUint64(&failureSOCKS, 1)
}

func IncSuccessForwardTCP() {
	atomic.AddUint64(&successFwdTCP, 1)
}

func IncFailureForwardTCP() {
	atomic.AddUint64(&failureFwdTCP, 1)
}

func IncSuccessForwardUDP() {
	atomic.AddUint64(&successFwdUDP, 1)
}

func IncFailureForwardUDP() {
	atomic.AddUint64(&failureFwdUDP, 1)
}

func RegisterConnection(remoteAddr, localAddr string) uint64 {
	id := atomic.AddUint64(&connID, 1)
	cs := &connStats{
		id:         id,
		remoteAddr: remoteAddr,
		localAddr:  localAddr,
		startedAt:  time.Now().UTC(),
	}
	cs.lastSeen.Store(time.Now().UTC())
	cs.closedAt.Store(time.Time{})
	connMu.Lock()
	conns[id] = cs
	connMu.Unlock()
	return id
}

func UnregisterConnection(id uint64) {
	now := time.Now().UTC()
	connMu.RLock()
	cs := conns[id]
	connMu.RUnlock()
	if cs == nil {
		return
	}
	cs.closedAt.Store(now)
	cs.lastSeen.Store(now)
}

func AddConnIngress(id uint64, bytes int) {
	if bytes <= 0 {
		return
	}
	connMu.RLock()
	cs := conns[id]
	connMu.RUnlock()
	if cs == nil {
		return
	}
	atomic.AddUint64(&cs.bytesIn, uint64(bytes))
	cs.lastSeen.Store(time.Now().UTC())
}

func AddConnEgress(id uint64, bytes int) {
	if bytes <= 0 {
		return
	}
	connMu.RLock()
	cs := conns[id]
	connMu.RUnlock()
	if cs == nil {
		return
	}
	atomic.AddUint64(&cs.bytesOut, uint64(bytes))
	cs.lastSeen.Store(time.Now().UTC())
}

func IncConnStreams(id uint64) {
	connMu.RLock()
	cs := conns[id]
	connMu.RUnlock()
	if cs == nil {
		return
	}
	atomic.AddInt64(&cs.activeStreams, 1)
	cs.lastSeen.Store(time.Now().UTC())
}

func DecConnStreams(id uint64) {
	connMu.RLock()
	cs := conns[id]
	connMu.RUnlock()
	if cs == nil {
		return
	}
	atomic.AddInt64(&cs.activeStreams, -1)
	cs.lastSeen.Store(time.Now().UTC())
}

func SnapshotNow() Snapshot {
	uptime := time.Since(startTime).Seconds()
	if uptime == 0 {
		uptime = 1
	}
	inBytes := atomic.LoadUint64(&ingressBytes)
	outBytes := atomic.LoadUint64(&egressBytes)
	roleValue, _ := role.Load().(string)
	if roleValue == "" {
		roleValue = "unknown"
	}
	now := time.Now().UTC()
	connMu.Lock()
	for id, cs := range conns {
		if closedAt, ok := cs.closedAt.Load().(time.Time); ok && !closedAt.IsZero() {
			if now.Sub(closedAt) > connectionRetention {
				delete(conns, id)
			}
		}
	}
	connSnaps := make([]ConnSnapshot, 0, len(conns))
	for _, cs := range conns {
		status := "active"
		var closedAtStr string
		var closedAt time.Time
		if closedAt, ok := cs.closedAt.Load().(time.Time); ok && !closedAt.IsZero() {
			status = "closed"
			closedAtStr = closedAt.Format(time.RFC3339Nano)
		}
		connUptime := time.Since(cs.startedAt).Seconds()
		if !closedAt.IsZero() {
			connUptime = closedAt.Sub(cs.startedAt).Seconds()
		}
		if connUptime == 0 {
			connUptime = 1
		}
		lastSeen, _ := cs.lastSeen.Load().(time.Time)
		uptimeSeconds := time.Since(cs.startedAt).Seconds()
		if !closedAt.IsZero() {
			uptimeSeconds = closedAt.Sub(cs.startedAt).Seconds()
		}
		connSnaps = append(connSnaps, ConnSnapshot{
			ID:             cs.id,
			RemoteAddr:     cs.remoteAddr,
			LocalAddr:      cs.localAddr,
			StartedAt:      cs.startedAt.Format(time.RFC3339Nano),
			LastSeen:       lastSeen.Format(time.RFC3339Nano),
			Status:         status,
			ClosedAt:       closedAtStr,
			UptimeSeconds:  int64(uptimeSeconds),
			BytesIn:        atomic.LoadUint64(&cs.bytesIn),
			BytesOut:       atomic.LoadUint64(&cs.bytesOut),
			BytesInPerSec:  float64(atomic.LoadUint64(&cs.bytesIn)) / connUptime,
			BytesOutPerSec: float64(atomic.LoadUint64(&cs.bytesOut)) / connUptime,
			ActiveStreams:  atomic.LoadInt64(&cs.activeStreams),
		})
	}
	connMu.Unlock()
	sort.Slice(connSnaps, func(i, j int) bool {
		return connSnaps[i].ID < connSnaps[j].ID
	})
	return Snapshot{
		Role:              roleValue,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
		UptimeSeconds:     int64(time.Since(startTime).Seconds()),
		BytesIn:           inBytes,
		BytesOut:          outBytes,
		PacketsIn:         atomic.LoadUint64(&ingressPackets),
		PacketsOut:        atomic.LoadUint64(&egressPackets),
		BytesInPerSec:     float64(inBytes) / uptime,
		BytesOutPerSec:    float64(outBytes) / uptime,
		ActiveConnections: atomic.LoadInt64(&activeConnections),
		ActiveStreams:     atomic.LoadInt64(&activeStreams),
		SuccessTotal:      atomic.LoadUint64(&successTotal),
		FailureTotal:      atomic.LoadUint64(&failureTotal),
		SuccessTCP:        atomic.LoadUint64(&successTCP),
		FailureTCP:        atomic.LoadUint64(&failureTCP),
		SuccessUDP:        atomic.LoadUint64(&successUDP),
		FailureUDP:        atomic.LoadUint64(&failureUDP),
		SuccessSOCKS:      atomic.LoadUint64(&successSOCKS),
		FailureSOCKS:      atomic.LoadUint64(&failureSOCKS),
		SuccessFwdTCP:     atomic.LoadUint64(&successFwdTCP),
		FailureFwdTCP:     atomic.LoadUint64(&failureFwdTCP),
		SuccessFwdUDP:     atomic.LoadUint64(&successFwdUDP),
		FailureFwdUDP:     atomic.LoadUint64(&failureFwdUDP),
		Connections:       connSnaps,
	}
}

func Handler(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			limit = n
		}
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 500 {
		limit = 500
	}
	snap := SnapshotNow()
	if len(snap.Connections) > limit {
		snap.Connections = snap.Connections[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(snap)
}
