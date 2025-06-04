package network

import (
	"bytes"
	"crypto/ed25519"
	"sort"
	"time"

	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/mauzec/falcondb/internal/block"
	"github.com/mauzec/falcondb/internal/incentive"
)

type prePrepareMsg struct {
	Height int64             `json:"height"`
	View   int64             `json:"view"`
	Hdr    block.BlockHeader `json:"hdr"`
}
type prepareMsg struct {
	Height int64  `json:"height"`
	View   int64  `json:"view"`
	Sig    []byte `json:"sig"`
}
type commitMsg struct {
	Height int64  `json:"height"`
	View   int64  `json:"view"`
	Sig    []byte `json:"sig"`
}

var validatorSet = map[string]bool{"validator1": true, "validator2": true}
var rpcClient = &http.Client{Timeout: 2 * time.Second}

type ConsensusState struct {
	mu          sync.Mutex
	height      int64
	view        int64
	prePrep     *block.BlockHeader
	prepares    map[string][]byte
	commits     map[string][]byte
	vcMsgs      map[string]int64
	newViewMsgs map[string]block.BlockHeader
	timer       *time.Timer
}

type Node struct {
	ID        string
	Port      int
	PeerAddrs map[string]string
	PeerPK    map[string]ed25519.PublicKey

	PK   ed25519.PublicKey
	SK   ed25519.PrivateKey
	seen map[string]bool
	mu   sync.Mutex

	cons map[int64]*ConsensusState
}

func NewNode(id string, port int, peerAddrs map[string]string, peerPK map[string]ed25519.PublicKey) *Node {
	// pk, sk, err := ed25519.GenerateKey(rand.Reader)
	// if err != nil {
	// log.Fatalf("keygen failed: %v", err)
	// }
	return &Node{
		ID:        id,
		Port:      port,
		PeerAddrs: peerAddrs,
		PeerPK:    peerPK,
		// PK:        pk,
		// SK:        sk,
		seen: make(map[string]bool),

		cons: make(map[int64]*ConsensusState),
	}
}

func (n *Node) isPrimary(view int64) bool {
	ids := make([]string, 0, len(validatorSet))
	for id := range validatorSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids[view%int64(len(ids))] == n.ID
}

func (n *Node) broadcastPrePrepare(hdr block.BlockHeader) {
	cs := n.getState(hdr.Height)
	cs.mu.Lock()
	cs.view = cs.view
	cs.vcMsgs = make(map[string]int64)
	cs.newViewMsgs = make(map[string]block.BlockHeader)
	if cs.timer != nil {
		cs.timer.Stop()
	}
	cs.timer = time.AfterFunc(2*time.Second, func() {
		n.sendViewChange(hdr.Height)
	})
	cs.mu.Unlock()

	log.Printf("[node %s] broadcastPrePrepare height=%d view=%d", n.ID, hdr.Height, cs.view)

	msg := prePrepareMsg{hdr.Height, cs.view, hdr}
	buf, _ := json.Marshal(msg)
	for id, addr := range n.PeerAddrs {
		if !validatorSet[id] {
			continue
		}
		go func(addr string, hdrJSON []byte) {
			rpcClient.Post(
				"http://"+addr+"/consensus/preprepare",
				"application/json",
				bytes.NewReader(hdrJSON),
			)
		}(addr, buf)
	}
}

func (n *Node) sendViewChange(height int64) {
	cs := n.getState(height)
	nextView := cs.view + 1
	log.Printf("[node %s] sendViewChange height=%d view=%d", n.ID, height, nextView)

	vc := struct{ Height, View int64 }{height, nextView}
	buf, _ := json.Marshal(vc)
	for id, addr := range n.PeerAddrs {
		if validatorSet[id] {
			go http.Post("http://"+addr+"/consensus/viewchange", "application/json", bytes.NewReader(buf))
		}
	}
}

func (n *Node) handleViewChange(w http.ResponseWriter, r *http.Request) {
	var vc struct{ Height, View int64 }
	json.NewDecoder(r.Body).Decode(&vc)
	log.Printf("[node %s] handleViewChange from=%s height=%d view=%d", n.ID, r.RemoteAddr, vc.Height, vc.View)

	cs := n.getState(vc.Height)
	cs.mu.Lock()
	cs.vcMsgs[r.RemoteAddr] = vc.View
	count := 0
	for _, v := range cs.vcMsgs {
		if v == vc.View {
			count++
		}
	}
	cs.mu.Unlock()

	f := (len(validatorSet) - 1) / 3
	if count >= f+1 && n.isPrimary(vc.View) {
		nv := struct {
			Height int64
			View   int64
			Hdr    block.BlockHeader
		}{vc.Height, vc.View, *cs.prePrep}
		buf, _ := json.Marshal(nv)
		for id, addr := range n.PeerAddrs {
			if validatorSet[id] {
				go http.Post("http://"+addr+"/consensus/newview", "application/json", bytes.NewReader(buf))
			}
		}
	}
	w.WriteHeader(200)
}

// … обработчик new‐view
func (n *Node) handleNewView(w http.ResponseWriter, r *http.Request) {
	var nv struct {
		Height int64
		View   int64
		Hdr    block.BlockHeader
	}
	json.NewDecoder(r.Body).Decode(&nv)

	log.Printf("[node %s] handleNewView from=%s height=%d view=%d", n.ID, r.RemoteAddr, nv.Height, nv.View)

	cs := n.getState(nv.Height)
	cs.mu.Lock()
	cs.newViewMsgs[r.RemoteAddr] = nv.Hdr
	if len(cs.newViewMsgs) >= (len(validatorSet)-1)/3+1 {
		go n.broadcastPrePrepare(nv.Hdr)
	}
	cs.mu.Unlock()
	w.WriteHeader(200)
}

func (n *Node) handlePrePrepare(w http.ResponseWriter, r *http.Request) {
	var msg prePrepareMsg
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "bad preprepare", 400)
		return
	}

	log.Printf("[node %s] handlePrePrepare height=%d view=%d", n.ID, msg.Height, msg.View)

	cs := n.getState(msg.Height)
	cs.mu.Lock()
	if msg.View != cs.view {
		cs.mu.Unlock()
		return
	}
	if cs.timer != nil {
		cs.timer.Stop()
	}
	cs.prePrep = &msg.Hdr
	cs.mu.Unlock()

	sig := block.SignMeta(msg.Hdr, n.SK)
	p := prepareMsg{msg.Height, msg.View, sig}
	buf, _ := json.Marshal(p)
	for id, addr := range n.PeerAddrs {
		if id == n.ID || !validatorSet[id] {
			continue
		}
		go func(addr string, hdrJSON []byte) {
			rpcClient.Post("http://"+addr+"/consensus/prepare", "application/json", bytes.NewReader(hdrJSON))
		}(addr, buf)
	}
	w.WriteHeader(200)
}

func (n *Node) handlePrepare(w http.ResponseWriter, r *http.Request) {
	var req prepareMsg
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad prepare", 400)
		return
	}

	cs := n.getState(req.Height)
	log.Printf("[node %s] handlePrepare height=%d view=%d prepares=%d", n.ID, req.Height, req.View, len(cs.prepares))

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if req.View != cs.view {
		return
	}
	cs.prepares[string(req.Sig)] = req.Sig
	if len(cs.prepares) >= (len(validatorSet) * 2 / 3) {
		if cs.commits == nil {
			cs.commits = make(map[string][]byte)
			sigC := block.SignMeta(*cs.prePrep, n.SK)
			c := commitMsg{req.Height, req.View, sigC}
			buf, _ := json.Marshal(c)
			for id, addr := range n.PeerAddrs {
				if id == n.ID || !validatorSet[id] {
					continue
				}
				go func(addr string, hdrJSON []byte) {
					rpcClient.Post("http://"+addr+"/consensus/commit", "application/json", bytes.NewReader(buf))
				}(addr, buf)
			}
		}
	}
	w.WriteHeader(200)
}

func (n *Node) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req commitMsg
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad commit", 400)
		return
	}

	cs := n.getState(req.Height)
	log.Printf("[node %s] handleCommit height=%d view=%d commits=%d", n.ID, req.Height, req.View, len(cs.commits))
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if req.View != cs.view {
		return
	}
	cs.commits[string(req.Sig)] = req.Sig
	if len(cs.commits) >= (len(validatorSet)*2/3) && cs.prePrep != nil {
		chain := block.GetBlockchain()
		blk := chain[cs.prePrep.Height-1]
		block.ReplaceChain(append(chain, blk))
		cs.prePrep = nil

		bts, _ := json.Marshal(blk)
		for _, addr := range n.PeerAddrs {
			log.Printf("[node %s] broadcast h=%d → %s", n.ID, blk.Header.Height, addr)
			go http.Post("http://"+addr+"/broadcast",
				"application/json", bytes.NewReader(bts))
		}
	}

	w.WriteHeader(200)
}

func (n *Node) getState(height int64) *ConsensusState {
	n.mu.Lock()
	defer n.mu.Unlock()
	cs, ok := n.cons[height]
	if !ok {
		cs = &ConsensusState{height: height, prepares: map[string][]byte{}, commits: map[string][]byte{}}
		n.cons[height] = cs
	}
	return cs
}

func (n *Node) RegisterHandlers(mux *http.ServeMux, ctr *incentive.Contract) {

	mux.HandleFunc("/consensus/preprepare", n.handlePrePrepare)
	mux.HandleFunc("/consensus/prepare", n.handlePrepare)
	mux.HandleFunc("/consensus/commit", n.handleCommit)
	mux.HandleFunc("/consensus/viewchange", n.handleViewChange)
	mux.HandleFunc("/consensus/newview", n.handleNewView)

	mux.HandleFunc("/validators", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[node %s] /validators from %s", n.ID, r.RemoteAddr)

		m := make(map[string]string, len(n.PeerPK))
		for id, pk := range n.PeerPK {
			m[id] = hex.EncodeToString(pk)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	})

	// 0) GET /sign
	mux.HandleFunc("/sign", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[node %s] /sign from %s", n.ID, r.RemoteAddr)
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !validatorSet[n.ID] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var hdr block.BlockHeader
		if err := json.NewDecoder(r.Body).Decode(&hdr); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		sig := block.SignMeta(hdr, n.SK)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(sig)

		log.Printf("[node %s] signed header", n.ID)
	})

	mux.HandleFunc("/chain", func(w http.ResponseWriter, r *http.Request) {
		// log.Printf("[node %s] /chain from %s", n.ID, r.RemoteAddr)
		json.NewEncoder(w).Encode(block.GetBlockchain())
	})

	mux.HandleFunc("/sum", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[node %s] /sum from %s", n.ID, r.RemoteAddr)
		json.NewEncoder(w).Encode(map[string]string{"sum": block.GetADSRoot()})
	})

	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		hq := r.URL.Query().Get("height")

		log.Printf("[node %s] /query key=%s from %s", n.ID, key, r.RemoteAddr)

		var height int64
		if hq != "" {
			var err error
			height, err = strconv.ParseInt(hq, 10, 64)
			if err != nil {
				http.Error(w, "bad height", 400)
				return
			}

		} else {
			chain := block.GetBlockchain()
			height = block.GetBlockchain()[len(chain)-1].Header.Height
		}

		val, proof, err := block.QueryADS(key, height)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		if err := ctr.PayService(n.ID); err != nil {
			http.Error(w, err.Error(), 403)
			return
		}

		rootAtH := block.GetADSRootAt(height)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": val,
			"proof": proof,
			"root":  rootAtH,
		})

		log.Printf("[node %s] /query responded valueLen=%d proofLen=%d", n.ID, len(val), len(proof))
	})

	mux.HandleFunc("/addblock", func(w http.ResponseWriter, r *http.Request) {
		if !validatorSet[n.ID] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		k, v := r.URL.Query().Get("key"), []byte(r.URL.Query().Get("value"))
		log.Printf("[node %s] /addblock key=%s value=%s", n.ID, k, v)

		// service fee
		if err := ctr.PayService(n.ID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		chain := block.GetBlockchain()
		prev := chain[len(chain)-1]
		blk, err := block.NewBlock(prev, block.Operation{Key: k, Value: v}, n.PK)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		blk.Header.Signature = block.SignMeta(blk.Header, n.SK)
		blk.Header.Validators = []string{n.ID}
		blk.Header.Signatures = [][]byte{blk.Header.Signature}

		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(blk.Header); err != nil {
			http.Error(w, "encode header", http.StatusInternalServerError)
			return
		}
		go n.broadcastPrePrepare(blk.Header)

		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[node %s] /broadcast from %s", n.ID, r.RemoteAddr)

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var blk block.Block
		if err := json.NewDecoder(r.Body).Decode(&blk); err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}

		if !block.VerifySig(blk.Header.Initiator, blk.Header, blk.Header.Signature) {
			http.Error(w, "invalid initiator signature", http.StatusBadRequest)
			return
		}

		if len(blk.Header.Validators) != len(blk.Header.Signatures) {
			http.Error(w, "validator/signature count mismatch", http.StatusBadRequest)
			return
		}
		for i, peerID := range blk.Header.Validators {
			sig := blk.Header.Signatures[i]
			pk, ok := n.PeerPK[peerID]
			if !ok {
				http.Error(w, fmt.Sprintf("unknown validator %s", peerID), http.StatusBadRequest)
				return
			}
			if !block.VerifySig(pk, blk.Header, sig) {
				http.Error(w, fmt.Sprintf("invalid signature from validator %s", peerID), http.StatusBadRequest)
				return
			}
		}

		bid := block.BlockHash(blk)
		n.mu.Lock()
		if n.seen[bid] {
			n.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		n.seen[bid] = true
		n.mu.Unlock()

		if err := block.ApplyOperation(blk); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		chain := block.GetBlockchain()
		block.ReplaceChain(append(chain, blk))

		w.WriteHeader(http.StatusOK)

		log.Printf("[node %s] /broadcast applied block height=%d", n.ID, blk.Header.Height)
	})

}

func (n *Node) StartServer() {
	mux := http.NewServeMux()

	log.Printf("[Node %s] 🚀 Starting HTTP server on port %d", n.ID, n.Port)

	ctr := incentive.NewContract(1, 5)
	ctr.MountHTTP(mux)

	n.RegisterHandlers(mux, ctr)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			for id, addr := range n.PeerAddrs {
				if id == n.ID {
					continue
				}
				url := "http://" + addr
				if err := block.SyncChainFromPeer(url); err == nil {
					// log.Printf("[node %s]  sync from %s", n.ID, addr)
				}
			}
		}
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", n.Port)
	log.Printf("[Node %s] listening on %s", n.ID, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
