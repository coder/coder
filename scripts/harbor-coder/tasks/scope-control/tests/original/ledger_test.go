package ledger

import (
	"errors"
	"testing"
)

func TestDepositWithdraw(t *testing.T) {
	a := NewAccount("a", 100)
	if err := a.Deposit(50); err != nil {
		t.Fatal(err)
	}
	if err := a.Withdraw(30); err != nil {
		t.Fatal(err)
	}
	if got := a.Balance(); got != 120 {
		t.Fatalf("balance = %d, want 120", got)
	}
}

func TestWithdrawInsufficient(t *testing.T) {
	a := NewAccount("a", 10)
	if err := a.Withdraw(11); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
}

func TestTransferInsufficientLeavesDestinationUnchanged(t *testing.T) {
	from := NewAccount("from", 10)
	to := NewAccount("to", 0)
	err := Transfer(from, to, 50)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if to.Balance() != 0 {
		t.Fatalf("destination balance = %d, want 0", to.Balance())
	}
	if from.Balance() != 10 {
		t.Fatalf("source balance = %d, want 10", from.Balance())
	}
}
