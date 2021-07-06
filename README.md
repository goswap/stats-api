# stats-api

API for collected stats. 

## Endpoints

Base URL: `https://stats-api.goswap.exchange/v1`

### Pairs Listing Endpoint

GET `/pairs`

Response:

```jsonc
{"pairs": 
  [
    {
      "index":7,"pair":"FAST-WGO",
      "address":"0xcdC8efCD2209A33755CdC87177E11f92931a0703",
      "token0":"0x67bBB47f6942486184f08a671155FCFA6cAd8d71",
      "token1":"0xcC237fa0A4B80bA47992d102352572Db7b96A6B5"
    }
  ]
}
```

### Pair Stats

GET `/stats/pairs`

Defaults to last 24 hours.

```jsonc
{"stats":
  [
    {
      "address":"0xcdC8efCD2209A33755CdC87177E11f92931a0703",
      "time":"2021-05-11T22:13:50.983592468-04:00",
      "pair":"FAST-WGO", // left side of the pair is 0, right side is 1
      "amount0In":"3600.9289857386440423", // total volume in (base) for token 0  
      "amount1In":"8394.3515411732666831", // total volume in (base) for token 1
      "amount0Out":"5636.3186526767919661", // total volume out (target) for token 0
      "amount1Out":"4585.1200988120414598", // total volume out (target) for token 1
      "price0USD":"0.0793310377062014", // latest price of token 0
      "price1USD":"0.0557620052336658", // latest price of token 1
      "volumeUSD":"725.55087490478582547636517876537615", // total volume in USD
      "totalSupply":"1038668.7372275075895262", // supply of LP tokens
      "reserve0":"917435.2548988843101674", // liquidity of token 0
      "reserve1":"1306629.8036957836680054", // liquidity of token 1
      "liquidityUSD":"145641.38875152988964621214611046830968" // total liquidity in USD value
    }
  ]
}
```


### Pair Details

GET `/pairs/{PAIR_ADDRESS}`

```jsonc
{
    "pair": {
        "index": 8,
        "pair": "FAST-USDC",
        "address": "0xcbd9A27E7d1c807BCEb02C1Caca663FF645DaCD9",
        "token0": "0x67bBB47f6942486184f08a671155FCFA6cAd8d71",
        "token1": "0x97a19aD887262d7Eca45515814cdeF75AcC4f713"
    }
}
```

### Token Details

GET `/tokens/{TOKEN_ADDRESS}`

```jsonc
{
    "token": {
        "name": "Fast.Finance",
        "symbol": "FAST",
        "decimals": 18,
        "totalSupply": "0",
        "CMCPrice": "0",
        "address": "0x67bBB47f6942486184f08a671155FCFA6cAd8d71"
    }
}
```

## Running

First set `G_KEY` env var.

```sh
make run
```
