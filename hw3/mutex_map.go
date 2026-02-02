package main

import (
	"fmt"
	"sync"
	"time"
)

type SafeMap struct {
	mu sync.Mutex
	m  map[int]int
}

func (sm *SafeMap) Set(key, value int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

func main() {
	safeMap := SafeMap{m: make(map[int]int)}
	var wg sync.WaitGroup

	start := time.Now()

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				safeMap.Set(g*1000+i, i)
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Println("Map length:", len(safeMap.m))
	fmt.Println("Time taken:", elapsed)
}

