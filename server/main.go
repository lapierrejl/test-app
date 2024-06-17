package main

import (
    "context"
    "encoding/json"
    "log"
    "math"
    "net/http"
    "runtime"
    "sync"
    "time"
)

var (
    stressWg  sync.WaitGroup
    stressCtx sync.Map
    isRunning bool
    mu        sync.Mutex
)

type StressRequest struct {
    CPURequest     *int `json:"cpu_request"`
    CPUPercent     *int `json:"cpu_percent"`
    MemoryRequestMB *int `json:"memory_request_mb"`
    MemoryPercent  *int `json:"memory_percent"`
    Duration        int  `json:"duration"`
}

type StressResponse struct {
    Message string `json:"message"`
}

func main() {
    http.HandleFunc("/stress", stressHandler)
    http.HandleFunc("/stop", stopHandler)
    http.HandleFunc("/healthz", healthzHandler)
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func stressHandler(w http.ResponseWriter, r *http.Request) {
    log.Printf("Received stress request from %s", r.RemoteAddr)

    if r.Method != http.MethodPost {
        log.Printf("Invalid request method: %s", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    mu.Lock()
    if isRunning {
        log.Printf("Stress test is already running")
        http.Error(w, "Stress test is already running", http.StatusConflict)
        mu.Unlock()
        return
    }
    isRunning = true
    mu.Unlock()

    var stressReq StressRequest
    err := json.NewDecoder(r.Body).Decode(&stressReq)
    if err != nil {
        log.Printf("Failed to decode request body: %v", err)
        http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
        mu.Lock()
        isRunning = false
        mu.Unlock()
        return
    }

    var cpuMillicores, memoryBytes int
    if stressReq.CPURequest != nil && stressReq.CPUPercent != nil {
        if *stressReq.CPURequest <= 0 {
            http.Error(w, "CPU request must be greater than 0", http.StatusBadRequest)
            mu.Lock()
            isRunning = false
            mu.Unlock()
            return
        }
        if *stressReq.CPUPercent < 0 || *stressReq.CPUPercent > 100 {
            http.Error(w, "CPU percentage must be between 0 and 100", http.StatusBadRequest)
            mu.Lock()
            isRunning = false
            mu.Unlock()
            return
        }
        cpuMillicores = int(float64(*stressReq.CPURequest) * float64(*stressReq.CPUPercent) / 100)
    }

    if stressReq.MemoryRequestMB != nil && stressReq.MemoryPercent != nil {
        if *stressReq.MemoryRequestMB <= 0 {
            http.Error(w, "Memory request must be greater than 0", http.StatusBadRequest)
            mu.Lock()
            isRunning = false
            mu.Unlock()
            return
        }
        if *stressReq.MemoryPercent < 0 || *stressReq.MemoryPercent > 100 {
            http.Error(w, "Memory percentage must be between 0 and 100", http.StatusBadRequest)
            mu.Lock()
            isRunning = false
            mu.Unlock()
            return
        }
        memoryBytes = int(float64(*stressReq.MemoryRequestMB) * 1024 * 1024 * float64(*stressReq.MemoryPercent) / 100)
    }

    log.Printf("Starting stress test with parameters: CPU=%d millicores, Memory=%d bytes, Duration=%d seconds",
        cpuMillicores, memoryBytes, stressReq.Duration)

    ctx, cancel := context.WithCancel(context.Background())
    stressCtx.Store(r.RemoteAddr, cancel)

    stressWg.Add(1)
    go func() {
        defer func() {
            stressWg.Done()
            mu.Lock()
            isRunning = false
            mu.Unlock()
            log.Printf("Stress test completed")
        }()
        generateLoad(ctx, cpuMillicores, memoryBytes, stressReq.Duration)
    }()

    response := StressResponse{
        Message: "Stress test started",
    }

    jsonResponse, err := json.Marshal(response)
    if err != nil {
        http.Error(w, "Failed to generate JSON response", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(jsonResponse)
    log.Printf("Stress test response sent")
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    cancel, ok := stressCtx.Load(r.RemoteAddr)
    if !ok {
        http.Error(w, "No stress test is currently running", http.StatusConflict)
        return
    }

    cancel.(context.CancelFunc)()

    stressWg.Wait()

    mu.Lock()
    isRunning = false
    mu.Unlock()

    log.Printf("Stress test stopped")

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Stress test stopped successfully"))
}

func generateLoad(ctx context.Context, cpuMillicores int, memoryBytes int, duration int) {
    log.Printf("Starting stress test: CPU=%d millicores, Memory=%d bytes, Duration=%d seconds",
        cpuMillicores, memoryBytes, duration)

    runtime.GOMAXPROCS(runtime.NumCPU())

    var memoryChunk []byte
    if memoryBytes > 0 {
        memoryChunk = make([]byte, memoryBytes)
        log.Printf("Allocated memory chunk of size %d bytes", memoryBytes)
    }

    done := make(chan bool)
    go func() {
        select {
        case <-ctx.Done():
            log.Printf("Context canceled, stopping stress test")
        case <-time.After(time.Duration(duration) * time.Second):
            log.Printf("Stress test duration of %d seconds elapsed", duration)
        }
        done <- true
    }()

    startTime := time.Now()
    iterations := 1000000

    for {
        select {
        case <-done:
            log.Printf("Stress test completed")
            return
        case <-ctx.Done():
            log.Printf("Context canceled, stopping stress test")
            return
        default:
            if cpuMillicores > 0 {
                log.Printf("Starting CPU stress test")
                cpuStressTest(done, startTime, iterations, cpuMillicores, duration)
                log.Printf("CPU stress test completed")
            }

            if memoryBytes > 0 {
                log.Printf("Starting memory stress test")
                _ = memoryChunk
                log.Printf("Memory stress test completed")
            }

            currentTime := time.Now()
            remainingMillisInSecond := 1000 - (currentTime.Sub(startTime).Milliseconds() % 1000)
            time.Sleep(time.Duration(remainingMillisInSecond) * time.Millisecond)
        }
    }
}

func cpuStressTest(done <-chan bool, startTime time.Time, iterations int, cpuMillicores int, duration int) {
    currentTime := time.Now()
    elapsedMillis := currentTime.Sub(startTime).Milliseconds()
    targetMillis := int64(cpuMillicores) * elapsedMillis / 1000
    remainingMillis := int64(duration)*1000 - elapsedMillis

    if remainingMillis <= 0 {
        return
    }

    adjustedIterations := iterations * int(remainingMillis) / int(targetMillis)

    for i := 0; i < adjustedIterations; i++ {
        select {
        case <-done:
            return
        default:
            cpuIntensiveTask()
        }
    }

    actualMillis := time.Since(startTime).Milliseconds()
    if actualMillis < targetMillis {
        iterations *= 2
    } else if actualMillis > targetMillis+1000 {
        iterations /= 2
    }
}

func cpuIntensiveTask() {
    _ = math.Sqrt(1234567890)
}