package utils

import (
	"math/big"

	"github.com/shopspring/decimal"
)

func DecToInt(d decimal.Decimal, decimals int) *big.Int {
	// multiply amount by number of decimals
	d1 := decimal.New(1, int32(decimals))
	d = d.Mul(d1)
	i := &big.Int{}
	i.SetString(d.StringFixed(0), 10)
	return i
}

func IntToDec(i *big.Int, decimals uint8) decimal.Decimal {
	d := decimal.NewFromBigInt(i, 0)
	d = d.Div(decimal.New(1, int32(decimals)))
	return d
}
