package main

import (
	"fmt"
	"runtime"
	"time"
)

func main() {
	roundTrips := 1000000

	// Single-threaded (GOMAXPROCS = 1)
	fmt.Println("===== SINGLE THREAD (GOMAXPROCS=1) =====")
	runtime.GOMAXPROCS(1)
	singleTime := pingPong(roundTrips)
	singleAvg := float64(singleTime.Nanoseconds()) / float64(roundTrips*2)
	fmt.Printf("Total time:  %v\n", singleTime)
	fmt.Printf("Avg switch:  %.2f ns\n", singleAvg)

	// Multi-threaded (GOMAXPROCS = default/all CPUs)
	fmt.Println("\n===== MULTI THREAD (GOMAXPROCS=default) =====")
	runtime.GOMAXPROCS(runtime.NumCPU())
	multiTime := pingPong(roundTrips)
	multiAvg := float64(multiTime.Nanoseconds()) / float64(roundTrips*2)
	fmt.Printf("Total time:  %v\n", multiTime)
	fmt.Printf("Avg switch:  %.2f ns\n", multiAvg)

	// Comparison
	fmt.Println("\n===== COMPARISON =====")
	fmt.Printf("Single-thread avg: %.2f ns\n", singleAvg)
	fmt.Printf("Multi-thread avg:  %.2f ns\n", multiAvg)
}

func pingPong(n int) time.Duration {
	ping := make(chan int) // Channel that carries integers
	pong := make(chan int) // Channel that carries integers

	// Goroutine A: Waits for ping, sends pong
	go func() {
		for i := 0; i < n; i++ {
			msg := <-ping   // Receive number from ping
			pong <- msg + 1 // Send number + 1 back through pong
		}
	}()

	start := time.Now()

	// Main Goroutine: Sends ping, waits for pong
	for i := 0; i < n; i++ {
		ping <- i // Send number i through ping channel
		<-pong    // Wait to receive response from pong channel
	}

	return time.Since(start)
}
