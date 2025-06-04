package light

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mauzec/falcondb/internal/block"
)

type LightClient struct {
	Headers []block.BlockHeader

	ADSRoot string
	Server  string

	PeerPK map[string]ed25519.PublicKey
}

func NewLightClient(serverURL string) (*LightClient, error) {
	lc := &LightClient{Server: serverURL}

	resp, err := http.Get(serverURL + "/validators")
	if err != nil {
		return nil, fmt.Errorf("fetch validators: %w", err)
	}
	defer resp.Body.Close()
	var pkHex map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&pkHex); err != nil {
		return nil, fmt.Errorf("decode validators: %w", err)
	}
	lc.PeerPK = make(map[string]ed25519.PublicKey, len(pkHex))
	for id, h := range pkHex {
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil, fmt.Errorf("bad pubkey hex for %s: %w", id, err)
		}
		lc.PeerPK[id] = ed25519.PublicKey(b)
	}

	resp, err = http.Get(serverURL + "/chain")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chain []block.Block
	if err := json.NewDecoder(resp.Body).Decode(&chain); err != nil {
		return nil, err
	}
	for _, blk := range chain {
		if err := lc.processHeader(blk.Header); err != nil {
			return nil, fmt.Errorf("invalid chain: %w", err)
		}
	}

	lc.ADSRoot = fmt.Sprintf("%x", chain[len(chain)-1].Header.DataHash)
	return lc, nil
}

func (lc *LightClient) processHeader(h block.BlockHeader) error {
	n := len(lc.Headers)
	if n > 0 {
		prev := lc.Headers[n-1]
		if h.Height != prev.Height+1 {
			return fmt.Errorf("height mismatch: %d vs %d", prev.Height+1, h.Height)
		}
		if !bytes.Equal(h.PrevHash, block.HashHeader(prev)) {
			return fmt.Errorf("prevHash mismatch at height %d", h.Height)
		}
	}

	if len(h.Validators) != len(h.Signatures) {
		return fmt.Errorf("validator/signature count mismatch")
	}
	for i, sig := range h.Signatures {
		vid := h.Validators[i]
		pk, ok := lc.PeerPK[vid]
		if !ok {
			return fmt.Errorf("unknown validator %s", vid)
		}
		if !block.VerifySig(pk, h, sig) {
			return fmt.Errorf("invalid signature from %s", vid)
		}
	}

	lc.Headers = append(lc.Headers, h)
	return nil
}

func (lc *LightClient) SyncOne() error {
	nextH := lc.Headers[len(lc.Headers)-1].Height + 1
	resp, err := http.Get(fmt.Sprintf("%s/chain", lc.Server))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var chain []block.Block
	if err := json.NewDecoder(resp.Body).Decode(&chain); err != nil {
		return err
	}
	if len(chain) < int(nextH) {
		return nil
	}
	return lc.processHeader(chain[nextH-1].Header)
}

func (lc *LightClient) Query(key string) ([]byte, error) {
	h := lc.Headers[len(lc.Headers)-1].Height
	u := fmt.Sprintf("%s/query?key=%s&height=%d", lc.Server, key, h)
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s", string(body))
	}
	var out struct {
		Value []byte   `json:"value"`
		Proof [][]byte `json:"proof"`
		Root  string   `json:"root"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Root != lc.ADSRoot {
		return nil, fmt.Errorf("root mismatch: local=%s got=%s", lc.ADSRoot, out.Root)
	}
	if !block.VerifyQry(out.Root, key, out.Value, out.Proof) {
		return nil, fmt.Errorf("proof verification failed")
	}
	return out.Value, nil
}
