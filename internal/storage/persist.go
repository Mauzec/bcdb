package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
)

var adsDB *leveldb.DB

func init() {
	if os.Getenv("MODE") == "client" {
		log.Printf("[ads-persist] skip LevelDB open (client mode)")
		return
	}
	path := os.Getenv("ADS_PATH")
	log.Printf("[ads-persist] Opening ADS DB at %s", path)
	var err error
	adsDB, err = leveldb.OpenFile(path, nil)
	if err != nil {
		log.Fatalf("[ads-persist] cannot open ads.db: %v", err)
	}
	iter := adsDB.NewIterator(nil, nil)
	if !iter.Next() {
		genHash := sha256.Sum256([]byte("genesis"))
		v := Version{
			Value: genHash[:],
			VF:    0,
			VT:    InfVT,
		}
		raw, _ := json.Marshal(v)
		key := fmt.Sprintf("ver:__genesis__:%s", padVF(0))
		if err := adsDB.Put([]byte(key), raw, nil); err != nil {
			log.Fatalf("[ads-persist] seed genesis: %v", err)
		}
	}
	iter.Release()
}

func NewADS() *ADS {
	if adsDB == nil {
		return NewMemADS()
	}
	data := make(map[string][]Version)
	iter := adsDB.NewIterator(nil, nil)
	for iter.Next() {
		key := string(iter.Key()) // "ver:{k}:{vf}"
		parts := strings.Split(key, ":")
		k, vf := parts[1], parseVF(parts[2])

		if k == "__genesis__" {
			continue
		}
		var v Version
		json.Unmarshal(iter.Value(), &v)
		v.VF = vf
		data[k] = append(data[k], v)
	}
	iter.Release()
	// сортируем по VF
	for k := range data {
		sort.Slice(data[k], func(i, j int) bool {
			return data[k][i].VF < data[k][j].VF
		})
	}
	return &ADS{Data: data}
}

func padVF(vf int64) string {
	return fmt.Sprintf("%020d", vf)
}

func parseVF(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func NewMemADS() *ADS {
	return &ADS{
		Data: make(map[string][]Version),
	}
}
