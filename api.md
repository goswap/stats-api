# GOSwap Stats API

TODO document all fields
TODO sort / pagination ?

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
apart, between `time_start` and `time_end`. These are returned in
chronological order.

```
/v1/stats
?time_frame=1h REQUIRED
?time_start=RFC3339-date REQUIRED
?time_end=RFC3339-date REQUIRED
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
volume, amountIn and amountOut returned will be summed over the given time
range for each token, priceUSD, reserve and liquidityUSD will be the latest
values. Tokens with no activity in the given time window will not be returned.
The results are returned in no particular order.

```
/v1/stats/tokens
?time_start=RFC3339-date REQUIRED
?time_end=RFC3339-date REQUIRED
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
?time_frame=1h REQUIRED
?time_start=RFC3339-date REQUIRED
?time_end=RFC3339-date REQUIRED
```

return token stats for a single token between `time_start` and `time_end` that
are `time_frame` apart. Results returned in chronological order.

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
?time_start=RFC3339-date REQUIRED
?time_end=RFC3339-date REQUIRED
```

return pair stats across all pairs between `time_start` and `time_end`, the
volume, amountIn and amountOut returned will be summed over the given time
range for each pair, priceUSD, reserve, totalSupply and liquidityUSD will be
the latest values. Pairs with no activity in the given time window will not be
returned.  The results are returned in no particular order.


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
?time_frame=1h REQUIRED
?time_start=RFC3339-date REQUIRED
?time_end=RFC3339-date REQUIRED
```

return pair stats for a single token between `time_start` and `time_end` that
are `time_frame` apart. Results returned in chronological order.

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
