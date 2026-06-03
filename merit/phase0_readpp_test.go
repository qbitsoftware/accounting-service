package merit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPhase0ReadPrepayments(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	time.Sleep(2 * time.Second) // let async batch settle

	windows := [][2]string{
		{"20261201", "20270228"}, // around 2027-01-15
		{"20260501", "20260731"}, // around 2026-06-03
		{"20250501", "20250731"}, // around 2025-06-02
	}
	for _, w := range windows {
		// no filter
		got, err := c.ListPayments(ctx, ListPaymentsParams{PeriodStart: w[0], PeriodEnd: w[1]})
		if err != nil {
			t.Logf("window %s..%s ERROR: %v", w[0], w[1], err)
			continue
		}
		t.Logf("window %s..%s -> %d payments (no filter)", w[0], w[1], len(got))
		dump(t, fmt.Sprintf("PAYMENTS %s..%s", w[0], w[1]), got)
		// probe PaymentType buckets to see which one holds prepayments
		for pt := 0; pt <= 5; pt++ {
			ptv := pt
			g, err := c.ListPayments(ctx, ListPaymentsParams{PeriodStart: w[0], PeriodEnd: w[1], PaymentType: &ptv})
			if err == nil && len(g) > 0 {
				t.Logf("  PaymentType=%d -> %d", pt, len(g))
			}
		}
	}
}
