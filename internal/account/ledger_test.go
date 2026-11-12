package account

import "testing"

func TestSettleOpenMargin(t *testing.T) {
	tests := []struct {
		name            string
		frozenChange    int64
		abailableChange int64
		wantAvailable   int64
	}{
		{name: "at limit", frozenChange: -100, abailableChange: 0, wantAvailable: 900},
		{name: "price improvement", frozenChange: -100, abailableChange: 10, wantAvailable: 910},
		{name: "margin top up", frozenChange: -100, abailableChange: -10, wantAvailable: 890},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := NewLedger()
			if err := ledger.Deposit(1, "USDT", 1000); err != nil {
				t.Fatal(err)
			}
			if err := ledger.Freeze(1, "USDT", 100); err != nil {
				t.Fatal(err)
			}

			err := ledger.Settle(1, "USDT", test.frozenChange, test.abailableChange)
			if err != nil {
				t.Fatal(err)
			}

			balance, ok := ledger.Balance(1, "USDT")
			if !ok {
				t.Fatal("balance not found")
			}
			if balance.Available != test.wantAvailable || balance.Frozen != 0 {
				t.Fatalf("balance = %+v, want available %d and frozen 0", balance, test.wantAvailable)
			}
			if balance.AccountID != 1 || balance.Currency != "USDT" {
				t.Fatalf("balance identity = (%d,%q), want (1,USDT)", balance.AccountID, balance.Currency)
			}
		})
	}
}

func TestSettleRejectsNegativeBalancesAtomically(t *testing.T) {
	ledger := NewLedger()
	if err := ledger.Deposit(1, "USDT", 100); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Freeze(1, "USDT", 50); err != nil {
		t.Fatal(err)
	}

	if err := ledger.Settle(1, "USDT", -51, 0); err == nil {
		t.Fatal("Settle should reject an over-release of frozen funds")
	}
	if err := ledger.Settle(1, "USDT", -50, -51); err == nil {
		t.Fatal("Settle should reject an overdrawn available balance")
	}

	balance, _ := ledger.Balance(1, "USDT")
	if balance.Available != 50 || balance.Frozen != 50 {
		t.Fatalf("balance changed after failed settlement: %+v", balance)
	}
}
