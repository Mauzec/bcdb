package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"crypto/ed25519"

	"github.com/joho/godotenv"
	"github.com/mauzec/falcondb/internal/network"
)

func parsePKEnv(name string) ed25519.PublicKey {
	s := os.Getenv(name)
	s = strings.Trim(s, "[]")
	parts := strings.Fields(s)
	pk := make([]byte, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			log.Fatalf("bad %s byte %q: %v", name, p, err)
		}
		pk[i] = byte(v)
	}
	return ed25519.PublicKey(pk)
}

func parseSKEnv(name string) ed25519.PrivateKey {
	s := os.Getenv(name)
	s = strings.Trim(s, "[]")
	parts := strings.Fields(s)
	sk := make([]byte, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			log.Fatalf("bad %s byte %q: %v", name, p, err)
		}
		sk[i] = byte(v)
	}
	return ed25519.PrivateKey(sk)
}

func main() {

	if err := godotenv.Load("cmd/test/app.env"); err != nil {
		log.Fatalf("load .env: %v", err)
	}

	var (
		id   string
		port int
	)
	// var dataDir string
	// flag.StringVar(&dataDir, "data", "", "data directory for this node")
	flag.StringVar(&id, "id", "", "node id")
	flag.IntVar(&port, "port", 0, "HTTP port")
	flag.Parse()
	if id == "" || port == 0 {
		log.Fatalf("pass --id and --port")
	}

	configs := []struct {
		ID   string
		Port int
	}{
		{"validator1", 8081},
		{"validator2", 8082},
		{"node1", 8091},
		{"node2", 8092},
		{"node3", 8093},
	}
	peerAddrs := make(map[string]string, len(configs))
	for _, c := range configs {
		peerAddrs[c.ID] = fmt.Sprintf("127.0.0.1:%d", c.Port)
	}

	peerPK := map[string]ed25519.PublicKey{
		"validator1": parsePKEnv("VALIDATOR1PK"),
		"validator2": parsePKEnv("VALIDATOR2PK"),
		"node1":      parsePKEnv("NODE1PK"),
		"node2":      parsePKEnv("NODE2PK"),
		"node3":      parsePKEnv("NODE3PK"),
	}

	var sk ed25519.PrivateKey
	switch id {
	case "validator1":
		sk = parseSKEnv("VALIDATOR1SK")
	case "validator2":
		sk = parseSKEnv("VALIDATOR2SK")
	case "node1":
		sk = parseSKEnv("NODE1SK")
	case "node2":
		sk = parseSKEnv("NODE2SK")
	case "node3":
		sk = parseSKEnv("NODE3SK")
	default:
		log.Fatalf("unknown id %s", id)
	}

	n := network.NewNode(id, port, peerAddrs, peerPK)
	n.SK = sk
	log.Printf("Starting %s on :%d", id, port)
	n.StartServer()

}
