# stats-api

API for collected stats. 

## Building

Do not use `go mod vendor` for this project.
`cgo` dependencies require `hidapi.h` and `secp256k1.h` files which both not available in vendored variant 
because of https://github.com/golang/go/issues/26366.

## Running

First set `G_KEY` env var.

```sh
make run
```
