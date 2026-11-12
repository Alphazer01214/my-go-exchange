package marketdata

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// FixedScale 是外部十进制价格、数量转换为 int64 时使用的小数位数。
const FixedScale int64 = 100_000_000

// DecimalToFixed 将非负十进制字符串转成 1e8 定点 int64。
// 超过 8 位的小数只有在多余位全为 0 时才允许。
func DecimalToFixed(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, errors.New("empty decimal")
	}
	if s[0] == '-' {
		return 0, errors.New("negative decimal not supported: " + strconv.Quote(raw))
	}
	s = strings.TrimPrefix(s, "+")

	intPart, fracPart, found := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if found && fracPart == "" {
		return 0, errors.New("invalid decimal: " + strconv.Quote(raw))
	}
	const fracDigits = 8
	for len(fracPart) > fracDigits && strings.HasSuffix(fracPart, "0") {
		fracPart = fracPart[:len(fracPart)-1]
	}
	if len(fracPart) > fracDigits {
		return 0, errors.New("too many decimals: " + strconv.Quote(raw))
	}
	for len(fracPart) < fracDigits {
		fracPart += "0"
	}

	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("invalid decimal: " + strconv.Quote(raw))
	}
	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil || frac < 0 {
		return 0, errors.New("invalid decimal: " + strconv.Quote(raw))
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if whole > (maxInt64-frac)/FixedScale {
		return 0, errors.New("decimal overflows int64: " + strconv.Quote(raw))
	}
	return whole*FixedScale + frac, nil
}

// FloatToFixed 将非负有限 float64 四舍五入成 1e8 定点 int64。
func FloatToFixed(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	scaled := math.Round(value * float64(FixedScale))
	const maxInt64 = float64(^uint64(0) >> 1)
	if scaled > maxInt64 {
		return 0, false
	}
	return int64(scaled), true
}
