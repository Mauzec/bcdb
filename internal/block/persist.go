package block

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var blkDB *leveldb.DB
var BlkPath string

func init() {
	if os.Getenv("MODE") == "client" {
		log.Printf("[block-persist] skip LevelDB open (client mode)")
		return
	}

	path := os.Getenv("BLK_PATH")
	log.Printf("[block-persist] Opening blockchain DB at %s", path)
	var err error
	blkDB, err = leveldb.OpenFile(path, nil)
	if err != nil {
		log.Fatalf("[block-persist] cannot open blockchain.db: %v", err)
	}
	log.Printf("[block-persist] Blockchain DB opened")

	iter := blkDB.NewIterator(util.BytesPrefix([]byte("block:")), nil)
	if !iter.Next() {
		gen := GenesisBlock()
		key := fmt.Sprintf("block:%020d", gen.Header.Height)
		raw, _ := json.Marshal(gen)
		log.Printf("[block-persist] Seeding genesis block height=%d", gen.Header.Height)
		if err := blkDB.Put([]byte(key), raw, nil); err != nil {
			log.Fatalf("[block-persist] cannot seed genesis: %v", err)
		}
		log.Printf("[block-persist] Genesis seeded")
	}
	iter.Release()
}

func saveBlock(b Block) error {
	log.Printf("[persist] saveBlock height=%d", b.Header.Height)
	key := fmt.Sprintf("block:%020d", b.Header.Height)
	raw, err := json.Marshal(b)
	if err != nil {
		log.Printf("[persist] marshal error: %v", err)
		return err
	}
	return blkDB.Put([]byte(key), raw, nil)
}

func GetBlockchain() []Block {
	// log.Printf("[persist] Loading blockchain from DB")
	iter := blkDB.NewIterator(util.BytesPrefix([]byte("block:")), nil)
	defer iter.Release()

	var chain []Block
	for iter.Next() {
		var b Block
		if err := json.Unmarshal(iter.Value(), &b); err != nil {
			log.Printf("[persist] skip invalid block: %v", err)
			continue
		}
		chain = append(chain, b)
	}
	// log.Printf("[persist] Loaded chain length=%d", len(chain))
	return chain
}
