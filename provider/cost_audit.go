package provider

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
)

const (
	minChargeHours   = 1.0
	hourlyPrecision  = 4
	priceTolerance   = 0.01
)

func truncateFloat(amount float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Floor(amount*factor) / factor
}

func roundToPrecision(amount float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(amount*factor) / factor
}

type CostSession struct {
	ServiceID      int64
	PackageID      int64
	DailyPrice     float64
	OrderedAt      time.Time
	ActualCharge   *float64
	ChargeVerified bool
}

type CostReport struct {
	ServiceID      int64
	PackageID      int64
	DailyPrice     float64
	OrderedAt      time.Time
	CanceledAt     time.Time
	DurationHours  float64
	ExpectedCost   float64
	ExpectedRefund float64
	ActualCharge   *float64
	ActualRefund   *float64
	LedgerRefund   *float64
	Mismatch       bool
	Notes          []string
}

func (r *CostReport) NetCost() float64 {
	charge := r.DailyPrice
	if r.ActualCharge != nil {
		charge = *r.ActualCharge
	}
	refund := 0.0
	if r.ActualRefund != nil {
		refund = *r.ActualRefund
	}
	return charge - refund
}

type CostTracker struct {
	client   *SHCClient
	sessions map[int64]*CostSession
	logger   *log.Logger
}

func NewCostTracker(client *SHCClient) *CostTracker {
	return &CostTracker{
		client:   client,
		sessions: make(map[int64]*CostSession),
		logger:   log.New(log.Writer(), "shc.cost: ", log.LstdFlags),
	}
}

func (t *CostTracker) TrackOrder(ctx context.Context, serviceID, packageID int64, actualCharge *float64) *CostSession {
	dailyPrice := t.client.EstimateDailyCost(ctx, packageID)
	session := &CostSession{
		ServiceID:    serviceID,
		PackageID:    packageID,
		DailyPrice:   dailyPrice,
		OrderedAt:    time.Now().UTC(),
		ActualCharge: actualCharge,
	}
	t.sessions[serviceID] = session

	if actualCharge != nil {
		diff := math.Abs(*actualCharge - dailyPrice)
		session.ChargeVerified = diff <= priceTolerance
		if session.ChargeVerified {
			t.logger.Printf("order svc %d — charged $%.4f, expected $%.4f — OK", serviceID, *actualCharge, dailyPrice)
		} else {
			t.logger.Printf("order svc %d — CHARGE MISMATCH: charged $%.4f, expected $%.4f (diff $%+.4f)",
				serviceID, *actualCharge, dailyPrice, *actualCharge-dailyPrice)
		}
	} else {
		t.logger.Printf("tracking svc %d — expected $%.4f/day (balance diff unavailable)", serviceID, dailyPrice)
	}

	return session
}

func (t *CostTracker) UpdateCharge(serviceID int64, actualCharge float64) {
	session, ok := t.sessions[serviceID]
	if !ok {
		return
	}
	session.ActualCharge = &actualCharge
	diff := math.Abs(actualCharge - session.DailyPrice)
	session.ChargeVerified = diff <= priceTolerance
	if session.ChargeVerified {
		t.logger.Printf("order svc %d — charged $%.2f, expected $%.2f — OK", serviceID, actualCharge, session.DailyPrice)
	} else {
		t.logger.Printf("order svc %d — CHARGE MISMATCH: charged $%.2f, expected $%.2f (diff $%+.2f)",
			serviceID, actualCharge, session.DailyPrice, actualCharge-session.DailyPrice)
	}
}

func (t *CostTracker) AuditCancel(ctx context.Context, serviceID int64, actualRefund *float64) *CostReport {
	session, ok := t.sessions[serviceID]
	if !ok {
		t.logger.Printf("no tracked session for svc %d", serviceID)
		return nil
	}

	now := time.Now().UTC()
	duration := now.Sub(session.OrderedAt)
	hours := duration.Hours()

	hourlyRate := math.Round(session.DailyPrice/24*math.Pow(10, hourlyPrecision)) / math.Pow(10, hourlyPrecision)
	chargedHours := math.Max(hours, minChargeHours)
	rawCharge := chargedHours * hourlyRate
	expectedCost := truncateFloat(rawCharge, 2)
	expectedRefund := math.Max(session.DailyPrice-expectedCost, 0.0)

	report := &CostReport{
		ServiceID:      serviceID,
		PackageID:      session.PackageID,
		DailyPrice:     session.DailyPrice,
		OrderedAt:      session.OrderedAt,
		CanceledAt:     now,
		DurationHours:  hours,
		ExpectedCost:   expectedCost,
		ExpectedRefund: expectedRefund,
		ActualCharge:   session.ActualCharge,
		ActualRefund:   actualRefund,
	}

	if actualRefund != nil {
		diff := math.Abs(*actualRefund - expectedRefund)
		if diff > priceTolerance {
			ledgerRefund := t.client.LedgerRefund(ctx, serviceID)
			if ledgerRefund != nil && math.Abs(*ledgerRefund-expectedRefund) <= priceTolerance {
				report.Mismatch = false
				report.LedgerRefund = ledgerRefund
				report.Notes = append(report.Notes, "balance_diff_noisy_concurrent_activity")
				t.logger.Printf("cancel svc %d — balance diff $%.4f ≠ expected $%.4f, but per-VM ledger confirms $%.4f — concurrent activity, OK",
					serviceID, *actualRefund, expectedRefund, *ledgerRefund)
			} else if ledgerRefund != nil {
				report.Mismatch = true
				report.LedgerRefund = ledgerRefund
				report.Notes = append(report.Notes, "ledger_confirms_mismatch")
				t.logger.Printf("cancel svc %d — REFUND MISMATCH confirmed by ledger: balance diff $%.4f, ledger $%.4f, expected $%.4f",
					serviceID, *actualRefund, *ledgerRefund, expectedRefund)
			} else {
				report.Mismatch = true
				report.Notes = append(report.Notes, fmt.Sprintf("refund_diff_$%+.4f", *actualRefund-expectedRefund))
				t.logger.Printf("cancel svc %d — REFUND MISMATCH: refunded $%.4f, expected $%.4f (diff $%+.4f)",
					serviceID, *actualRefund, expectedRefund, *actualRefund-expectedRefund)
			}
		} else {
			t.logger.Printf("cancel svc %d — refunded $%.4f, expected $%.4f — OK", serviceID, *actualRefund, expectedRefund)
		}
	} else {
		report.Notes = append(report.Notes, "refund_diff_unavailable")
	}

	chargeStr := "?"
	if session.ActualCharge != nil {
		chargeStr = fmt.Sprintf("$%.4f", *session.ActualCharge)
	}
	refundStr := "?"
	if actualRefund != nil {
		refundStr = fmt.Sprintf("$%.4f", *actualRefund)
	}
	t.logger.Printf("session svc %d — %.1f hrs, charged %s, refunded %s, net $%.4f",
		serviceID, hours, chargeStr, refundStr, report.NetCost())

	return report
}

func (t *CostTracker) CurrentBurn(serviceID int64) float64 {
	session, ok := t.sessions[serviceID]
	if !ok {
		return 0
	}
	hours := time.Since(session.OrderedAt).Hours()
	hourlyRate := math.Round(session.DailyPrice/24*math.Pow(10, hourlyPrecision)) / math.Pow(10, hourlyPrecision)
	return truncateFloat(math.Max(hours, minChargeHours)*hourlyRate, 2)
}

func (t *CostTracker) SessionReport(serviceID int64) map[string]interface{} {
	session, ok := t.sessions[serviceID]
	if !ok {
		return nil
	}
	hours := time.Since(session.OrderedAt).Hours()
	hourlyRate := math.Round(session.DailyPrice/24*math.Pow(10, hourlyPrecision)) / math.Pow(10, hourlyPrecision)
	expectedCost := truncateFloat(math.Max(hours, minChargeHours)*hourlyRate, 2)

	report := map[string]interface{}{
		"service_id":           session.ServiceID,
		"package_id":           session.PackageID,
		"daily_price":          session.DailyPrice,
		"hourly_rate":          hourlyRate,
		"ordered_at":           session.OrderedAt.Format(time.RFC3339),
		"elapsed_hours":        hours,
		"current_expected_cost": expectedCost,
		"charge_verified":      session.ChargeVerified,
	}
	if session.ActualCharge != nil {
		report["actual_charge"] = *session.ActualCharge
	}
	return report
}
