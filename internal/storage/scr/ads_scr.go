package main

import (
	"encoding/json"
	"fmt"

	"github.com/mauzec/falcondb/internal/storage"
)

func main() {
	var r struct {
		Value []byte
		Proof [][]byte
	}
	json.Unmarshal([]byte(`$Q`), &r)
	fmt.Println("VerifyQry:", storage.VerifyQry("$DIGEST", "foo", r.Value, r.Proof))
}
