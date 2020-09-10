module github.com/goswap/stats-api

go 1.15

require (
	cloud.google.com/go/firestore v1.3.0
	cloud.google.com/go/storage v1.11.0 // indirect
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/treeder/firetils v0.0.8
	github.com/treeder/gcputils v0.0.34
	github.com/treeder/goapibase v0.0.5
	github.com/treeder/gotils v0.0.15
	github.com/goswap/collector v0.0.3
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/sys v0.0.0-20200909081042-eff7692f9009 // indirect
	golang.org/x/tools v0.0.0-20200909210914-44a2922940c2 // indirect
	google.golang.org/api v0.31.0
	google.golang.org/grpc v1.32.0 // indirect
)

// replace github.com/goswap/collector => ../collector
