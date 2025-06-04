package incentive

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type Contract struct {
	svcFee  int
	authFee int
	mu      sync.Mutex
	deposit map[string]int
	balance map[string]int
	frozen  map[string]bool
}

func NewContract(svcFee, authFee int) *Contract {
	return &Contract{
		svcFee:  svcFee,
		authFee: authFee,
		deposit: make(map[string]int),
		balance: make(map[string]int),
		frozen:  make(map[string]bool),
	}
}

func (c *Contract) MountHTTP(mux *http.ServeMux) {
	// /deposit?node=ID&amt=100
	mux.HandleFunc("/deposit", func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node")
		amt, _ := strconv.Atoi(r.URL.Query().Get("amt"))

		log.Printf("[incentive] Deposit node=%s amt=%d", node, amt)

		c.mu.Lock()
		c.deposit[node] += amt
		c.mu.Unlock()
		w.WriteHeader(200)
	})

	// challenge: /challenge?server=ID
	mux.HandleFunc("/challenge", func(w http.ResponseWriter, r *http.Request) {
		srv := r.URL.Query().Get("server")

		log.Printf("[incentive] Challenge for server=%s", srv)

		c.mu.Lock()
		c.frozen[srv] = true
		c.mu.Unlock()
		w.WriteHeader(200)
	})

	// submitProof: POST {"server":"ID","valid":true}
	mux.HandleFunc("/submitProof", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Server string `json:"server"`
			Valid  bool   `json:"valid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		log.Printf("[incentive] submitProof server=%s valid=%v", req.Server, req.Valid)

		c.mu.Lock()
		defer c.mu.Unlock()

		if req.Valid {
			c.balance[req.Server] += c.authFee
		} else {
			c.deposit[req.Server] = 0
			c.balance[req.Server] = 0
		}
		c.frozen[req.Server] = false

		w.WriteHeader(200)
	})

	// withdraw: /withdraw?node=ID
	mux.HandleFunc("/withdraw", func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node")
		log.Printf("[incentive] Withdraw request node=%s", node)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.frozen[node] {
			http.Error(w, "account frozen", 400)
			return
		}
		amt := c.deposit[node] + c.balance[node]
		c.deposit[node], c.balance[node] = 0, 0
		json.NewEncoder(w).Encode(map[string]int{"amount": amt})
	})
}

func (c *Contract) PayService(nodeID string) error {
	log.Printf("[incentive] PayService node=%s", nodeID)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen[nodeID] {
		return fmt.Errorf("account frozen")
	}
	c.balance[nodeID] += c.svcFee
	return nil
}
