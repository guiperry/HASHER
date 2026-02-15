module data-miner

go 1.23

toolchain go1.24.1

require (
	github.com/am-sokolov/go-spacy v0.0.0-20250919212123-1d3a142ac336
	github.com/apache/arrow/go/arrow v0.0.0-20211112161151-bc219186db40
	github.com/vbauerster/mpb/v8 v8.7.4
	go.etcd.io/bbolt v1.4.0
)

replace github.com/am-sokolov/go-spacy => ./spacy

require (
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/flatbuffers v2.0.0+incompatible // indirect
	github.com/klauspost/compress v1.13.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.8 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

require (
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/sys v0.29.0 // indirect
)
