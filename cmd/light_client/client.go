package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mauzec/falcondb/internal/light"
)

func main() {

	lc, err := light.NewLightClient("http://127.0.0.1:8081")
	if err != nil {
		log.Fatalf("failed to init light client: %v", err)
	}
	fmt.Println("Synced headers:", len(lc.Headers))

	val, err := lc.Query("foo")
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	fmt.Printf("foo = %s\n", string(val))

	for {
		time.Sleep(5 * time.Second)
		if err := lc.SyncOne(); err != nil {
			log.Printf("sync error: %v", err)
			continue
		}
		fmt.Println("New block height:", lc.Headers[len(lc.Headers)-1].Height)
	}
}
