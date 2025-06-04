package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

type ProofNode struct {
	Hash []byte `json:"hash"`
	Left bool   `json:"left"`
}

func main() {

	var s struct {
		Sum string `json:"sum"`
	}
	resp, _ := http.Get("http://127.0.0.1:8092/sum")
	json.NewDecoder(resp.Body).Decode(&s)
	resp.Body.Close()

	var r struct {
		Value []byte      `json:"value"`
		Proof []ProofNode `json:"proof"`
	}
	resp, _ = http.Get("http://127.0.0.1:8092/query?key=hey")
	json.NewDecoder(resp.Body).Decode(&r)
	resp.Body.Close()

	leaf := sha256.Sum256(append([]byte("hey"), r.Value...))
	curr := leaf[:]
	for _, p := range r.Proof {
		var sum [32]byte
		if p.Left {
			sum = sha256.Sum256(append(p.Hash, curr...))
		} else {
			sum = sha256.Sum256(append(curr, p.Hash...))
		}
		curr = sum[:]
	}

	ok := hex.EncodeToString(curr) == s.Sum
	fmt.Println("VerifyQry:", ok)
}
