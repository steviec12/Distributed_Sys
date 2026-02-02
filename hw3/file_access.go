package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

func main() {
	iterations := 100000
	data := "hello world\n"

	// Unbuffered Write
	fmt.Println("===== UNBUFFERED =====")
	f1, _ := os.Create("unbuffered.txt")
	start1 := time.Now()

	for i := 0; i < iterations; i++ {
		f1.Write([]byte(data))
	}

	f1.Close()
	elapsed1 := time.Since(start1)
	fmt.Println("Time taken:", elapsed1)

	// Buffered Write
	fmt.Println("\n===== BUFFERED =====")
	f2, _ := os.Create("buffered.txt")
	w := bufio.NewWriter(f2)
	start2 := time.Now()

	for i := 0; i < iterations; i++ {
		w.WriteString(data)
	}

	w.Flush()
	f2.Close()
	elapsed2 := time.Since(start2)
	fmt.Println("Time taken:", elapsed2)

	// Comparison
	fmt.Println("\n===== COMPARISON =====")
	fmt.Printf("Unbuffered: %v\n", elapsed1)
	fmt.Printf("Buffered:   %v\n", elapsed2)
	fmt.Printf("Speedup:    %.2fx faster\n", float64(elapsed1)/float64(elapsed2))

	// Cleanup
	os.Remove("unbuffered.txt")
	os.Remove("buffered.txt")
}

