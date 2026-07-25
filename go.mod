module github.com/kbuechl/audible-hardcover-sync

go 1.25.4

replace github.com/mstrhakr/go-audible => ./third_party/go-audible

require github.com/mstrhakr/go-audible v0.2.7

require golang.org/x/crypto v0.49.0 // indirect
