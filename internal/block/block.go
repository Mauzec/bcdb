package block

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/mauzec/falcondb/internal/storage"
)

type Operation struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

type BlockHeader struct {
	Height      int64    `json:"height"`
	PrevHash    []byte   `json:"prev_hash"`    // hash(h{height-1})
	ContentHash []byte   `json:"content_hash"` // (phi) hach(C)
	DataHash    []byte   `json:"data_hash"`    // (delta) ADS-root
	RWHash      []byte   `json:"rw_hash"`      // hash(RW log)
	Initiator   []byte   `json:"initiator"`    // (e0) who proposed
	Signature   []byte   `json:"signature"`    // (s0) initiatur's signature
	Validators  []string `json:"validators"`   // {e1...ek} peer ids
	Signatures  [][]byte `json:"signatures"`   // {s1...sk}
}

type Block struct {
	Header  BlockHeader `json:"header"`
	Content []byte      `json:"content"`
}

var (
	blockchain   []Block
	blockchainMu sync.RWMutex
	store        = storage.NewADS()
)

func init() {
	genesis := GenesisBlock()
	blockchain = []Block{genesis}
}

func NewBlock(prev Block, op Operation, initiator []byte) (Block, error) {
	log.Printf("[block] NewBlock: prevHeight=%d key=%s", prev.Header.Height, op.Key)

	content, _ := json.Marshal(op)

	phiSum := sha256.Sum256(content)
	deltaHex, err := store.UpdS(op.Key, op.Value, prev.Header.Height+1)
	if err != nil {
		log.Printf("[block] UpdS error: %v", err)
		return Block{}, err
	}
	dataHash, _ := hex.DecodeString(deltaHex)

	rwSum := sha256.Sum256(content)

	hdr := BlockHeader{
		Height:      prev.Header.Height + 1,
		PrevHash:    hashHeader(prev.Header),
		ContentHash: phiSum[:],
		DataHash:    dataHash,
		RWHash:      rwSum[:],
		Initiator:   initiator,

		// Signatures, Validators
		// will add later with consensus
	}

	// s0 = sign(s_k(e_0), M)
	// hdr.Signature = SignMeta(hdr, s_k)

	blk := Block{Header: hdr, Content: content}
	if err := saveBlock(blk); err != nil {
		return Block{}, err
	}
	log.Printf("[block] NewBlock created height=%d φ=%x δ=%x", blk.Header.Height, phiSum[:4], dataHash[:4])
	return blk, nil
}

// func GetBlockchain() []Block {
// 	blockchainMu.RLock()
// 	defer blockchainMu.RUnlock()
// 	return append([]Block(nil), blockchain...)
// }

func BlockHash(blk Block) string {
	h := sha256.Sum256(jsonHeader(blk.Header))
	return hex.EncodeToString(h[:])
}

func jsonHeader(h BlockHeader) []byte {
	b, _ := json.Marshal(h)
	return b
}
func hashHeader(h BlockHeader) []byte {
	sum := sha256.Sum256(jsonHeader(h))
	return sum[:]
}

// sign(s_k(e_0), M)
func SignMeta(h BlockHeader, sk ed25519.PrivateKey) []byte {
	core := struct {
		Height      int64  `json:"height"`
		PrevHash    []byte `json:"prev_hash"`
		ContentHash []byte `json:"content_hash"`
		DataHash    []byte `json:"data_hash"`
		RWHash      []byte `json:"rw_hash"`
		Initiator   []byte `json:"initiator"`
	}{
		h.Height, h.PrevHash, h.ContentHash, h.DataHash, h.RWHash, h.Initiator,
	}
	b, _ := json.Marshal(core)
	return ed25519.Sign(sk, b)
}
func VerifySig(pk ed25519.PublicKey, h BlockHeader, sig []byte) bool {
	core := struct {
		Height      int64  `json:"height"`
		PrevHash    []byte `json:"prev_hash"`
		ContentHash []byte `json:"content_hash"`
		DataHash    []byte `json:"data_hash"`
		RWHash      []byte `json:"rw_hash"`
		Initiator   []byte `json:"initiator"`
	}{
		h.Height, h.PrevHash, h.ContentHash, h.DataHash, h.RWHash, h.Initiator,
	}
	b, _ := json.Marshal(core)
	return ed25519.Verify(pk, b, sig)
}

func ApplyOperation(b Block) error {
	log.Printf("[block] ApplyOperation height=%d", b.Header.Height)
	var op Operation
	if err := json.Unmarshal(b.Content, &op); err != nil {
		log.Printf("[block] unmarshal op error: %v", err)
		return err
	}
	newDelta, err := store.UpdS(op.Key, op.Value, b.Header.Height)
	if err != nil {
		log.Printf("[block] UpdS error: %v", err)
		return err
	}
	if newDeltaStr := hex.EncodeToString(b.Header.DataHash); newDelta != newDeltaStr {
		err := fmt.Errorf("ADS root mismatch: want %s, got %s", newDeltaStr, newDelta)
		log.Printf("[block] %v", err)
		return err
	}
	log.Printf("[block] ApplyOperation success new delta=%s", newDelta)
	return nil
}

func GetADSRoot() string {
	chain := GetBlockchain()
	h := chain[len(chain)-1].Header.Height
	return store.SumAt(h)
}

func GetADSRootAt(h int64) string {
	return store.SumAt(h)
}

func QueryADS(key string, height int64) ([]byte, []storage.ProofNode, error) {
	return store.Qry(key, height)
}

func IsValidChain(chain []Block) (bool, error) {
	for i := 1; i < len(chain); i++ {
		prev, curr := chain[i-1], chain[i]
		if curr.Header.Height != prev.Header.Height+1 {
			return false, fmt.Errorf("invalid height at %d", i)
		}
		if !bytes.Equal(curr.Header.PrevHash, hashHeader(prev.Header)) {
			return false, fmt.Errorf("invalid prev hash at %d", i)
		}
	}
	return true, nil
}

func ReplaceChain(newChain []Block) (bool, error) {
	// log.Printf("[block] ReplaceChain: newLen=%d oldLen=%d", len(newChain), len(blockchain))

	blockchainMu.Lock()
	defer blockchainMu.Unlock()
	if len(newChain) <= len(blockchain) {
		return false, nil
	}
	valid, err := IsValidChain(newChain)
	if err != nil || !valid {
		return false, err
	}
	blockchain = newChain
	for _, b := range newChain {
		if err := saveBlock(b); err != nil {
			return false, err
		}
	}
	// log.Printf("[block] ReplaceChain success, chain replaced")
	return true, nil
}

func GenesisBlock() Block {
	content := []byte("genesis")
	ch := sha256.Sum256(content)

	return Block{
		Header: BlockHeader{
			Height:      1,
			PrevHash:    nil,
			ContentHash: ch[:],
			DataHash:    ch[:],
			RWHash:      ch[:],
			Initiator:   []byte("system"),
		},
		Content: content,
	}
}

func SyncChainFromPeer(url string) error {
	resp, err := http.Get(url + "/chain")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var peerChain []Block
	if err := json.NewDecoder(resp.Body).Decode(&peerChain); err != nil {
		return err
	}

	local := GetBlockchain()
	for i := len(local); i < len(peerChain); i++ {
		if err := ApplyOperation(peerChain[i]); err != nil {
			return fmt.Errorf("sync apply op: %w", err)
		}
	}

	_, err = ReplaceChain(peerChain)
	return err
}

func HashHeader(h BlockHeader) []byte {
	return hashHeader(h)
}

func VerifyQry(rootHex, key string, value []byte, proof [][]byte) bool {
	return storage.VerifyQry(rootHex, key, value, proof)
}
