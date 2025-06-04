package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type MerkleNode struct {
	Hash   []byte
	Left   *MerkleNode
	Right  *MerkleNode
	Parent *MerkleNode
}

const InfVT = int64(1<<63 - 1)

// Version хранит одну версию записи с временными метками
type Version struct {
	Value []byte
	VF    int64
	VT    int64
}

// ADS теперь хранит историю по каждому ключу
type ADS struct {
	Data          map[string][]Version
	Leaves        []*MerkleNode
	Root          *MerkleNode
	CurrentHeight int64
}
type ProofNode struct {
	Hash []byte `json:"hash"`
	Left bool   `json:"left"`
}

// func NewADS() *ADS {
// 	return &ADS{
// 		Data:   make(map[string][]byte),
// 		Leaves: []*MerkleNode{},
// 	}
// }

// return root hash (delta)
func (a *ADS) Sum() string {
	if a.Root == nil {
		return ""
	}
	return hex.EncodeToString(a.Root.Hash)
}

// server update
func (a *ADS) UpdS(key string, value []byte, height int64) (string, error) {
	vers := a.Data[key]
	if len(vers) > 0 {
		vers[len(vers)-1].VT = height
	}
	v := Version{Value: value, VF: height, VT: InfVT}
	a.Data[key] = append(a.Data[key], v)
	if adsDB != nil {
		dbKey := fmt.Sprintf("ver:%s:%s", key, padVF(height))
		raw, _ := json.Marshal(v)
		adsDB.Put([]byte(dbKey), raw, nil)
	}

	a.CurrentHeight = height
	a.buildTree()
	return a.Sum(), nil
}

func (a *ADS) UpdC(newDigest string) error {
	if a.Sum() != newDigest {
		return errors.New("digest mismatch")
	}
	return nil
}

func (a *ADS) Qry(key string, height int64) ([]byte, []ProofNode, error) {
	a.CurrentHeight = height
	a.buildTree()
	vers, ok := a.Data[key]
	if !ok {
		return nil, nil, errors.New("key not found")
	}
	for i := len(vers) - 1; i >= 0; i-- {
		v := vers[i]
		if v.VF <= height && height < v.VT {
			h := sha256.Sum256(append([]byte(key), v.Value...))
			leaf := a.findLeaf(h[:])
			if leaf == nil {
				return nil, nil, errors.New("leaf not found")
			}
			return v.Value, a.genProof(leaf), nil
		}
	}
	return nil, nil, errors.New("no active version at this height")
}

func VerifyQry(digest string, key string, value []byte, proof [][]byte) bool {
	leafHash := sha256.Sum256(append([]byte(key), value...))
	curr := leafHash[:]
	for _, sib := range proof {
		sum := sha256.Sum256(append(curr, sib...))
		curr = sum[:]
	}
	return hex.EncodeToString(curr) == digest
}

func (a *ADS) buildTree() {
	a.Leaves = nil
	seen := map[string]bool{}
	for key, vers := range a.Data {
		if key == "__genesis__" {
			continue
		}
		for _, v := range vers {
			if v.VF <= a.CurrentHeight && a.CurrentHeight < v.VT {
				h := sha256.Sum256(append([]byte(key), v.Value...))
				hs := hex.EncodeToString(h[:])
				if seen[hs] {
					continue
				}
				seen[hs] = true
				a.Leaves = append(a.Leaves, &MerkleNode{Hash: h[:]})
			}
		}
	}
	sort.Slice(a.Leaves, func(i, j int) bool {
		return hex.EncodeToString(a.Leaves[i].Hash) < hex.EncodeToString(a.Leaves[j].Hash)
	})
	nodes := a.Leaves
	for len(nodes) > 1 {
		var next []*MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			if i+1 == len(nodes) {
				next = append(next, nodes[i])
			} else {
				left, right := nodes[i], nodes[i+1]
				ph := sha256.Sum256(append(left.Hash, right.Hash...))
				parent := &MerkleNode{Hash: ph[:], Left: left, Right: right}
				left.Parent, right.Parent = parent, parent
				next = append(next, parent)
			}
		}
		nodes = next
	}
	if len(nodes) == 1 {
		a.Root = nodes[0]
	}
}
func (a *ADS) findLeaf(hash []byte) *MerkleNode {
	for _, l := range a.Leaves {
		if bytes.Equal(l.Hash, hash) {
			return l
		}
	}
	return nil
}

func (a *ADS) genProof(leaf *MerkleNode) []ProofNode {
	proof := make([]ProofNode, 0)
	for n := leaf; n.Parent != nil; n = n.Parent {
		if n.Parent.Left == n {
			proof = append(proof, ProofNode{Hash: n.Parent.Right.Hash, Left: false})
		} else {
			proof = append(proof, ProofNode{Hash: n.Parent.Left.Hash, Left: true})
		}
	}
	return proof
}

func (a *ADS) SumAt(h int64) string {
	a.CurrentHeight = h
	a.buildTree()
	if a.Root == nil {
		return ""
	}
	return hex.EncodeToString(a.Root.Hash)
}

type Record struct {
	Key   string
	Value []byte
}

func (a *ADS) Scan(prefix string, height int64) ([]Record, error) {
	a.CurrentHeight = height
	a.buildTree()

	var out []Record
	for k, vers := range a.Data {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		for i := len(vers) - 1; i >= 0; i-- {
			v := vers[i]
			if v.VF <= height && height < v.VT {
				out = append(out, Record{
					Key:   k,
					Value: v.Value,
				})
				break
			}
		}
	}
	return out, nil
}
