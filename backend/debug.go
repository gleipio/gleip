package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"
)

// MemStats holds memory statistics
type MemStats struct {
	Alloc        uint64  `json:"alloc"`
	TotalAlloc   uint64  `json:"totalAlloc"`
	Sys          uint64  `json:"sys"`
	NumGC        uint32  `json:"numGC"`
	PauseTotalNs uint64  `json:"pauseTotalNs"`
	NumGoroutine int     `json:"numGoroutine"`
	HeapAlloc    uint64  `json:"heapAlloc"`
	HeapSys      uint64  `json:"heapSys"`
	HeapIdle     uint64  `json:"heapIdle"`
	HeapInuse    uint64  `json:"heapInuse"`
	AllocMB      float64 `json:"allocMB"`
	SysMB        float64 `json:"sysMB"`
	HeapAllocMB  float64 `json:"heapAllocMB"`
	HeapSysMB    float64 `json:"heapSysMB"`
}

// startDebugServer starts a debug HTTP server on port 9091
func StartDebugServer() {
	// Only start in debug mode
	if runtime.GOOS == "windows" {
		go func() {
			http.HandleFunc("/debug/stats", handleDebugStats)
			http.HandleFunc("/debug/mem", handleMemStats)
			http.ListenAndServe(":9091", nil)
		}()
	}
}

// handleDebugStats returns debug stats
func handleDebugStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"time":       time.Now().Format(time.RFC3339),
		"goroutines": runtime.NumGoroutine(),
		"cgocalls":   runtime.NumCgoCall(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleMemStats returns memory stats
func handleMemStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := MemStats{
		Alloc:        m.Alloc,
		TotalAlloc:   m.TotalAlloc,
		Sys:          m.Sys,
		NumGC:        m.NumGC,
		PauseTotalNs: m.PauseTotalNs,
		NumGoroutine: runtime.NumGoroutine(),
		HeapAlloc:    m.HeapAlloc,
		HeapSys:      m.HeapSys,
		HeapIdle:     m.HeapIdle,
		HeapInuse:    m.HeapInuse,
		// Convert to MB for readability
		AllocMB:     float64(m.Alloc) / 1024 / 1024,
		SysMB:       float64(m.Sys) / 1024 / 1024,
		HeapAllocMB: float64(m.HeapAlloc) / 1024 / 1024,
		HeapSysMB:   float64(m.HeapSys) / 1024 / 1024,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// startMemoryMonitor starts a goroutine that logs memory usage periodically
func StartMemoryMonitor() {
	// Only monitor in debug mode
	if runtime.GOOS == "windows" {
		go func() {
			for {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				allocMB := float64(m.Alloc) / 1024 / 1024
				sysMB := float64(m.Sys) / 1024 / 1024
				fmt.Printf("[Memory] Alloc: %.2fMB, Sys: %.2fMB, NumGC: %d, Goroutines: %d\n",
					allocMB, sysMB, m.NumGC, runtime.NumGoroutine())
				time.Sleep(30 * time.Second)
			}
		}()
	}
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
