# GOSwap Stats API

TODO finish documenting descriptions
TODO document all fields
TODO make the actual API look like this (WARNING: it currently does not)

### list tokens

list tokens returns a list of all tokens supported by goswap and their
metadata

`/v1/tokens`

`
{
  "tokens": [
    {
      "name": "string",
      "symbol": "string",
      "decimals": 123,
      "address": "0xaddress"
    }
  ]
}
`

### get token

get token returns a token's metadata

`/v1/tokens/{address}`

`
{
  "token": {
    "name": "string",
    "symbol": "string",
    "decimals": 123,
    "address": "0xaddress"
  }
}
`

### list pairs

list pairs returns a list of all pairs supported by goswap and their
metadata

`/v1/pairs`

`
{
  "pairs": [
    {
      "index": 123,
      "pair": "SYMBOL-SYMBOL",
      "address": "0xaddress",
      "token0": "0xaddress",
      "token1": "0xaddress"
    }
  ]
}
`

### get pair

get pair returns a pair's metadata

`/v1/pairs/{address}`

`
{
  "pair": {
    "index": 123,
    "pair": "SYMBOL-SYMBOL",
    "address": "0xaddress",
    "token0": "0xaddress",
    "token1": "0xaddress"
  }
}
`

### list stats totals

list stats returns a sum of stat totals across all tokens/pairs that are `time_frame`
apart, between `time_start` and `time_end`.

TODO defaults for `time_` fields

```
/v1/stats/totals
?time_frame=1h
?time_start=RFC3339-date
?time_end=RFC3339-date
```

`
{
  "stats": [
    {
      "time":"RFC3339-date",
      "volumeUSD": "1.23",
      "liquidityUSD": "1.23"
    }
  ]
}
`

### get all token stats

return token stats across all tokens between `time_start` and `time_end`, the
volume, amountIn and amountOut returned will be summed over the given time range for each token,
priceUSD and liquidityUSD will be the latest values.

TODO defaults for `time_` fields

```
/v1/stats/tokens
?time_start=RFC3339-date
?time_end=RFC3339-date
```

```
{
  "stats": [
    {
      "address": "0xaddress",
      "time": "RFC3339-date",
      "symbol": "string",
      "amountIn": "1.23",
      "amountOut": "1.23",
      "priceUSD": "1.23",
      "volumeUSD": "1.23",
      "reserve": "1.23",
      "liquidityUSD": "1.23"
    }
  ]
}
```

### get single token stats

```
/v1/stats/tokens/{address}
?time_frame=1h
?time_start=RFC3339-date
?time_end=RFC3339-date
```

return token stats for a single token between `time_start` and `time_end` that
are `time_frame` apart.

TODO defaults for `time_` fields

```
{
  "stats": [
    {
      "address": "0xaddress",
      "time": "RFC3339-date",
      "symbol": "string",
      "amountIn": "1.23",
      "amountOut": "1.23",
      "priceUSD": "1.23",
      "volumeUSD": "1.23",
      "reserve": "1.23",
      "liquidityUSD": "1.23"
    }
  ]
}
```

### get all pair stats

```
/v1/stats/pairs
?time_start=RFC3339-date
?time_end=RFC3339-date
```

return pair stats across all pairs between `time_start` and `time_end`, the
volume, amountIn and amountOut returned will be summed over the given time range for each token,
priceUSD and liquidityUSD will be the latest values.

TODO defaults for `time_` fields

```
{
  "stats": [
    {
      "address": "0xaddress",
      "time": "RFC3339-time",
      "pair": "SYMBOL-SYMBOL",
      "amount0In": "1.23",
      "amount1In": "1.23",
      "amount0Out": "1.23",
      "amount1Out": "1.23",
      "price0USD": "1.23",
      "price1USD": "1.23",
      "volumeUSD": "1.23",
      "totalSupply": "1.23",
      "reserve0": "1.23",
      "reserve1": "1.23",
      "liquidityUSD": "1.23"
    }
  ]
}
```

### Get single pair stats

```
/v1/stats/pairs/{address}
?time_frame=1h
?time_start=RFC3339-date
?time_end=RFC3339-date
```

return pair stats for a single token between `time_start` and `time_end` that
are `time_frame` apart.

TODO defaults for `time_` fields

```
{
  "stats": [
    {
      "address": "0xaddress",
      "time": "RFC3339-time",
      "pair": "SYMBOL-SYMBOL",
      "amount0In": "1.23",
      "amount1In": "1.23",
      "amount0Out": "1.23",
      "amount1Out": "1.23",
      "price0USD": "1.23",
      "price1USD": "1.23",
      "volumeUSD": "1.23",
      "totalSupply": "1.23",
      "reserve0": "1.23",
      "reserve1": "1.23",
      "liquidityUSD": "1.23"
    }
  ]
}
```
