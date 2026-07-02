package provider

import (
	"math"
	"testing"
	"time"
)

func TestTruncateFloat(t *testing.T) {
	tests := []struct {
		input    float64
		decimals int
		expected float64
	}{
		{0.0192, 2, 0.01},
		{0.019167, 2, 0.01},
		{0.107712, 2, 0.10},
		{0.99, 2, 0.99},
		{1.999, 2, 1.99},
		{0.0, 2, 0.0},
	}
	for _, tt := range tests {
		got := truncateFloat(tt.input, tt.decimals)
		if got != tt.expected {
			t.Errorf("truncateFloat(%v, %d) = %v, want %v", tt.input, tt.decimals, got, tt.expected)
		}
	}
}

func TestProrationFormula(t *testing.T) {
	daily := 0.46
	hourly := roundToPrecision(daily/24, hourlyPrecision)

	tests := []struct {
		hours         float64
		expectedCost  float64
		expectedRefund float64
	}{
		{1.0, 0.01, 0.45},
		{1.2, 0.02, 0.44},
		{3.0, 0.05, 0.41},
		{6.0, 0.11, 0.35},
		{0.5, 0.01, 0.45},
	}

	for _, tt := range tests {
		chargedHours := max(tt.hours, minChargeHours)
		rawCharge := chargedHours * hourly
		cost := truncateFloat(rawCharge, 2)
		refund := daily - cost

		if math.Abs(cost-tt.expectedCost) > 1e-9 {
			t.Errorf("hours=%v: cost=%.4f, want %.4f (raw=%.6f, hourly=%.6f)", tt.hours, cost, tt.expectedCost, rawCharge, hourly)
		}
		if math.Abs(refund-tt.expectedRefund) > 1e-9 {
			t.Errorf("hours=%v: refund=%.4f, want %.4f", tt.hours, refund, tt.expectedRefund)
		}
	}
}

func TestCostTrackerTrackOrder(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	charge := 0.49
	session := tracker.TrackOrder(nil, 123, 26, &charge)

	if session.ServiceID != 123 {
		t.Errorf("expected serviceID=123, got %d", session.ServiceID)
	}
	if session.ActualCharge == nil || *session.ActualCharge != 0.49 {
		t.Errorf("expected actualCharge=0.49, got %v", session.ActualCharge)
	}
}

func TestCostTrackerTrackOrderNoCharge(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	session := tracker.TrackOrder(nil, 123, 26, nil)

	if session.ActualCharge != nil {
		t.Errorf("expected nil ActualCharge, got %v", *session.ActualCharge)
	}
	if session.ChargeVerified {
		t.Error("expected ChargeVerified=false when no charge data")
	}
}

func TestCostTrackerUpdateCharge(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	tracker.TrackOrder(nil, 123, 26, nil)
	tracker.UpdateCharge(123, 0.49)

	session := tracker.sessions[123]
	if session.ActualCharge == nil || *session.ActualCharge != 0.49 {
		t.Errorf("expected charge=0.49, got %v", session.ActualCharge)
	}
}

func TestCostTrackerUpdateChargeUnknownService(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	tracker.UpdateCharge(999, 0.49)

	if _, exists := tracker.sessions[999]; exists {
		t.Error("should not create session for unknown service")
	}
}

func TestCostTrackerAuditCancelNoSession(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	report := tracker.AuditCancel(nil, 999, nil)
	if report != nil {
		t.Error("expected nil report for untracked session")
	}
}

func TestCostTrackerSessionReport(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	charge := 0.49
	tracker.TrackOrder(nil, 123, 26, &charge)

	report := tracker.SessionReport(123)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report["service_id"].(int64) != 123 {
		t.Errorf("expected serviceID=123, got %v", report["service_id"])
	}
}

func TestCostTrackerSessionReportUnknown(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	report := tracker.SessionReport(999)
	if report != nil {
		t.Error("expected nil report for unknown service")
	}
}

func TestCostTrackerCurrentBurn(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	session := &CostSession{
		ServiceID:    123,
		PackageID:    26,
		DailyPrice:   0.49,
		OrderedAt:    time.Now().UTC().Add(-3 * time.Hour),
	}
	tracker.sessions[123] = session

	burn := tracker.CurrentBurn(123)
	hourly := roundToPrecision(0.49/24, hourlyPrecision)
	expected := truncateFloat(3*hourly, 2)
	if burn != expected {
		t.Errorf("expected burn=%.4f, got %.4f", expected, burn)
	}
}

func TestCostTrackerCurrentBurnUnknown(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	burn := tracker.CurrentBurn(999)
	if burn != 0 {
		t.Errorf("expected 0 for unknown service, got %.4f", burn)
	}
}

func TestCostReportNetCost(t *testing.T) {
	charge := 0.49
	refund := 0.37
	report := &CostReport{
		DailyPrice:    0.49,
		ActualCharge:  &charge,
		ActualRefund:  &refund,
	}
	expected := 0.49 - 0.37
	if report.NetCost() != expected {
		t.Errorf("expected net=%.4f, got %.4f", expected, report.NetCost())
	}
}

func TestCostReportNetCostNoActuals(t *testing.T) {
	report := &CostReport{
		DailyPrice: 0.49,
	}
	if report.NetCost() != 0.49 {
		t.Errorf("expected net=0.49 (daily fallback), got %.4f", report.NetCost())
	}
}

func TestCostTrackerAuditCancelMatching(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	daily := 0.49
	session := &CostSession{
		ServiceID:    123,
		PackageID:    26,
		DailyPrice:   daily,
		OrderedAt:    time.Now().UTC().Add(-6 * time.Hour),
		ActualCharge: &daily,
	}
	tracker.sessions[123] = session

	hourly := roundToPrecision(daily/24, hourlyPrecision)
	expectedCost := truncateFloat(6*hourly, 2)
	expectedRefund := daily - expectedCost

	actualRefund := expectedRefund
	report := tracker.AuditCancel(nil, 123, &actualRefund)

	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Mismatch {
		t.Error("expected no mismatch when refund matches")
	}
	if report.ExpectedRefund != expectedRefund {
		t.Errorf("expected refund=%.4f, got %.4f", expectedRefund, report.ExpectedRefund)
	}
}

func TestCostTrackerAuditCancelMismatch(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	daily := 0.49
	session := &CostSession{
		ServiceID:    123,
		PackageID:    26,
		DailyPrice:   daily,
		OrderedAt:    time.Now().UTC().Add(-6 * time.Hour),
		ActualCharge: &daily,
	}
	tracker.sessions[123] = session

	wrongRefund := 0.01
	report := tracker.AuditCancel(nil, 123, &wrongRefund)

	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if !report.Mismatch {
		t.Error("expected mismatch when refund doesn't match")
	}
}

func TestCostTrackerAuditCancelNoRefundData(t *testing.T) {
	client := NewSHCClient("test-key", "")
	tracker := client.costTracker

	daily := 0.49
	session := &CostSession{
		ServiceID:    123,
		PackageID:    26,
		DailyPrice:   daily,
		OrderedAt:    time.Now().UTC().Add(-1 * time.Hour),
		ActualCharge: &daily,
	}
	tracker.sessions[123] = session

	report := tracker.AuditCancel(nil, 123, nil)

	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Mismatch {
		t.Error("expected no mismatch when refund data unavailable")
	}
	if len(report.Notes) == 0 || report.Notes[0] != "refund_diff_unavailable" {
		t.Errorf("expected 'refund_diff_unavailable' note, got %v", report.Notes)
	}
}

func TestSafeCreditReturnsNegativeOnError(t *testing.T) {
	client := NewSHCClient("invalid-key", "http://localhost:1")
	credit := client.SafeCredit(nil)
	if credit >= 0 {
		t.Errorf("expected negative credit on error, got %.2f", credit)
	}
}
