package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mauzec/falcondb/internal/block"
	"github.com/mauzec/falcondb/internal/storage"
)

func main() {

	log.Println("MODE:", os.Getenv("MODE"))

	server := "http://127.0.0.1:8081"

	var before struct {
		Sum string `json:"sum"`
	}
	resp, err := http.Get(server + "/sum")
	if err != nil {
		log.Fatal(err)
	}
	must(resp, &before)
	fmt.Println("🍃 old root =", before.Sum)

	var addRes struct {
		Block  block.Block `json:"block"`
		Digest string      `json:"digest"`
	}
	resp, err = http.Get(server + "/addblock?key=hey&value=bar")
	if err != nil {
		log.Fatal(err)
	}
	must(resp, &addRes)
	fmt.Println("new height=", addRes.Block.Header.Height, "digest=", addRes.Digest)

	resp, err = http.Get(server + "/query?key=hey")
	if err != nil {
		log.Fatal(err)
	}
	var q struct {
		Value []byte              `json:"value"`
		Proof []storage.ProofNode `json:"proof"`
	}
	json.NewDecoder(resp.Body).Decode(&q)
	resp.Body.Close()

	if verifyProof(addRes.Digest, "hey", q.Value, q.Proof) {
		fmt.Println("proof validated")
	} else {
		fmt.Println("proof FAILED")
		os.Exit(1)
	}
}

func verifyProof(digest, key string, val []byte, proof []storage.ProofNode) bool {
	curr := sha256.Sum256(append([]byte(key), val...))
	for _, p := range proof {
		if p.Left {
			curr = sha256.Sum256(append(p.Hash, curr[:]...))
		} else {
			curr = sha256.Sum256(append(curr[:], p.Hash...))
		}
	}
	return hex.EncodeToString(curr[:]) == digest
}

func must(resp *http.Response, dst interface{}) {
	if resp.StatusCode != 200 {
		log.Fatalf("bad status %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(dst)
	resp.Body.Close()
}
