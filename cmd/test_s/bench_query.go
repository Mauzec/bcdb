package main

import (
	"flag"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

func main() {
	var (
		server      = flag.String("server", "http://127.0.0.1:8081", "base URL of node")
		key         = flag.String("key", "foo", "key to query")
		runs        = flag.Int("runs", 5, "number of benchmark runs")
		requests    = flag.Int("n", 1000, "requests per run")
		concurrency = flag.Int("c", 50, "parallelism level")
	)
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Second}

	var totalLatencies []time.Duration
	var totalDuration time.Duration

	for r := 1; r <= *runs; r++ {
		latCh := make(chan time.Duration, *requests)
		errCh := make(chan error, *requests)
		sem := make(chan struct{}, *concurrency)
		var wg sync.WaitGroup

		start := time.Now()
		for i := 0; i < *requests; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				t0 := time.Now()
				resp, err := client.Get(fmt.Sprintf("%s/query?key=%s", *server, *key))
				dur := time.Since(t0)
				if err != nil {
					errCh <- err
				} else {
					resp.Body.Close()
					latCh <- dur
				}
				<-sem
			}()
		}
		wg.Wait()
		runDuration := time.Since(start)
		close(latCh)
		close(errCh)

		var lats []time.Duration
		for d := range latCh {
			lats = append(lats, d)
		}

		var errCount int
		for err := range errCh {
			_ = err
			errCount++
		}

		if len(lats) == 0 {
			fmt.Printf("Run %d: all %d requests failed\n", r, *requests)
			totalDuration += runDuration
			continue
		}

		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })

		sum := time.Duration(0)
		for _, d := range lats {
			sum += d
		}
		avg := sum / time.Duration(len(lats))
		p50 := lats[len(lats)/2]
		p95 := lats[int(float64(len(lats))*0.95)]
		p99 := lats[int(float64(len(lats))*0.99)]

		fmt.Printf("Run %d: duration=%v throughput=%.2f req/s, errors=%d\n",
			r, runDuration, float64(len(lats))/runDuration.Seconds(), errCount)
		fmt.Printf("  avg= %v, p50= %v, p95= %v, p99= %v, max= %v\n",
			avg, p50, p95, p99, lats[len(lats)-1])

		totalLatencies = append(totalLatencies, avg)
		totalDuration += runDuration
	}

	if len(totalLatencies) == 0 {
		fmt.Println("\nAll runs failed: no successful requests to report.")
		return
	}

	tot := time.Duration(0)
	for _, a := range totalLatencies {
		tot += a
	}
	avgLat := tot / time.Duration(len(totalLatencies))
	avgThroughput := float64(*requests*(*runs)) / totalDuration.Seconds()

	fmt.Printf("\nOverall avg latency: %v, avg throughput: %.2f req/s\n",
		avgLat, avgThroughput,
	)
}
