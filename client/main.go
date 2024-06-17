package main

import (
    "bytes"
	"context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "log"
    "net/http"

    "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

type StressRequest struct {
    CPURequest     *int `json:"cpu_request"`
    CPUPercent     *int `json:"cpu_percent"`
    MemoryRequestMB *int `json:"memory_request_mb"`
    MemoryPercent  *int `json:"memory_percent"`
    Duration        int  `json:"duration"`
}

func main() {
    deploymentName := flag.String("deployment", "", "Name of the deployment")
    namespace := flag.String("namespace", "default", "Namespace of the deployment")
    cpuRequest := flag.Int("cpu-request", 0, "CPU request in millicores")
    cpuPercent := flag.Int("cpu-percent", 0, "CPU utilization percentage")
    memoryRequestMB := flag.Int("memory-request", 0, "Memory request in megabytes")
    memoryPercent := flag.Int("memory-percent", 0, "Memory utilization percentage")
    duration := flag.Int("duration", 60, "Stress test duration in seconds")
    stopStress := flag.Bool("stop", false, "Stop the stress test")

    flag.Parse()

    if *deploymentName == "" {
        log.Fatal("Deployment name is required")
    }

    config, err := rest.InClusterConfig()
    if err != nil {
        log.Fatal(err)
    }

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatal(err)
    }

    pods, err := clientset.CoreV1().Pods(*namespace).List(context.TODO(), v1.ListOptions{
        LabelSelector: fmt.Sprintf("app=%s", *deploymentName),
    })
    if err != nil {
        log.Fatal(err)
    }

    if len(pods.Items) == 0 {
        log.Fatalf("No pods found for deployment %s in namespace %s", *deploymentName, *namespace)
    }

    for _, pod := range pods.Items {
        podIP := pod.Status.PodIP
        if podIP == "" {
            log.Printf("Skipping pod %s: no IP assigned", pod.Name)
            continue
        }

        if *stopStress {
            stopStressTest(podIP)
        } else {
            stressRequest := &StressRequest{
                CPURequest:     cpuRequest,
                CPUPercent:     cpuPercent,
                MemoryRequestMB: memoryRequestMB,
                MemoryPercent:  memoryPercent,
                Duration:       *duration,
            }
            startStressTest(podIP, stressRequest)
        }
    }
}

func startStressTest(podIP string, stressRequest *StressRequest) {
    url := fmt.Sprintf("http://%s:8080/stress", podIP)
    requestBody, err := json.Marshal(stressRequest)
    if err != nil {
        log.Printf("Failed to marshal stress request: %v", err)
        return
    }

    resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
    if err != nil {
        log.Printf("Failed to send stress request to pod %s: %v", podIP, err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Printf("Stress request to pod %s failed with status: %s", podIP, resp.Status)
        return
    }

    log.Printf("Stress test started on pod %s", podIP)
}

func stopStressTest(podIP string) {
    url := fmt.Sprintf("http://%s:8080/stop", podIP)
    resp, err := http.Post(url, "", nil)
    if err != nil {
        log.Printf("Failed to send stop request to pod %s: %v", podIP, err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        log.Printf("Stop request to pod %s failed with status: %s, body: %s", podIP, resp.Status, string(body))
        return
    }

    log.Printf("Stress test stopped on pod %s", podIP)
}