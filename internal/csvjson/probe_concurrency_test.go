package csvjson

import (
	"sync"
	"testing"
)

func TestProbeConcurrentInfer(t *testing.T) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 2000; j++ {
				_ = Infer("1e3")
				_ = Infer("plain")
			}
		}()
	}
	close(start)
	wg.Wait()
}
