package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	// Regular Integer (Non-Atomic)
	fmt.Println("===== REGULAR INTEGER =====")
	var regularOps int
	var wg1 sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			for j := 0; j < 1000; j++ {
				regularOps++
			}
		}()
	}

	wg1.Wait()
	fmt.Println("Expected: 50000")
	fmt.Println("Actual:  ", regularOps)
	fmt.Println()

	// Atomic Integer
	fmt.Println("===== ATOMIC INTEGER =====")
	var atomicOps atomic.Uint64
	var wg2 sync.WaitGroup

	for range 50 {
		wg2.Go(func() {
			for range 1000 {
				atomicOps.Add(1)
			}
		})
	}

	wg2.Wait()
	fmt.Println("Expected: 50000")
	fmt.Println("Actual:  ", atomicOps.Load())
}
