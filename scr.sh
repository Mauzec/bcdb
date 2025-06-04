rm -rf data/val1 data/val2 data/node1 data/node2 data/node3

ADS_PATH=data/val1/ads.db BLK_PATH=data/val1/blockchain.db \
  go run cmd/test/main.go --id=validator1 --port=8081

ADS_PATH=data/val2/ads.db BLK_PATH=data/val2/blockchain.db \
  go run cmd/test/main.go --id=validator2 --port=8082

ADS_PATH=data/node1/ads.db BLK_PATH=data/node1/blockchain.db \
  go run cmd/test/main.go --id=node1 --port=8091
ADS_PATH=data/node2/ads.db BLK_PATH=data/node2/blockchain.db \
  go run cmd/test/main.go --id=node2 --port=8092
ADS_PATH=data/node3/ads.db BLK_PATH=data/node3/blockchain.db \
  go run cmd/test/main.go --id=node3 --port=8093