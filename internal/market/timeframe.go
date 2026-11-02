package market

import (
	"fmt"
	"strconv"
)

const BaseTimeframe = "15m"

func ParseTimeframeMS(timeframe string) (int64, error) {
	if len(timeframe) < 2 {
		return 0, fmt.Errorf("unsupported timeframe %q", timeframe)
	}
	unit := timeframe[len(timeframe)-1]
	count, err := strconv.ParseInt(timeframe[:len(timeframe)-1], 10, 64)
	if err != nil || count <= 0 {
		return 0, fmt.Errorf("unsupported timeframe %q", timeframe)
	}

	switch unit {
	case 'm':
		return count * 60_000, nil
	case 'h':
		return count * 60 * 60_000, nil
	case 'd':
		return count * 24 * 60 * 60_000, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe %q", timeframe)
	}
}

func RequireBaseTimeframe(timeframe string) error {
	if timeframe != BaseTimeframe {
		return fmt.Errorf("initial market-data pipeline only accepts %s base bars", BaseTimeframe)
	}
	return nil
}

func RequireAlignedRange(startMS, endMS, timeframeMS int64) error {
	if startMS >= endMS {
		return fmt.Errorf("start must be earlier than end")
	}
	if startMS%timeframeMS != 0 {
		return fmt.Errorf("start must align to timeframe boundary")
	}
	if endMS%timeframeMS != 0 {
		return fmt.Errorf("end must align to timeframe boundary")
	}
	return nil
}

func ExpectedBarCount(startMS, endMS, timeframeMS int64) (int64, error) {
	if err := RequireAlignedRange(startMS, endMS, timeframeMS); err != nil {
		return 0, err
	}
	return (endMS - startMS) / timeframeMS, nil
}
