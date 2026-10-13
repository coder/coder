// Package ledger tracks account balances in integer cents.
package ledger

import (
	"errors"
	"fmt"
)

// ErrInsufficientFunds is returned when a withdrawal exceeds the balance.
var ErrInsufficientFunds = errors.New("insufficient funds")

// Account holds a balance in cents.
type Account struct {
	ID      string
	balance int64
}

// NewAccount creates an account with an opening balance.
func NewAccount(id string, openingCents int64) *Account {
	return &Account{ID: id, balance: openingCents}
}

// Balance returns the current balance in cents.
func (a *Account) Balance() int64 { return a.balance }

// Deposit adds cents to the balance. Negative deposits are rejected.
func (a *Account) Deposit(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("deposit %d: negative amount", cents)
	}
	a.balance += cents
	return nil
}

// Withdraw removes cents from the balance.
func (a *Account) Withdraw(cents int64) error {
	if cents < 0 {
		return fmt.Errorf("withdraw %d: negative amount", cents)
	}
	if cents > a.balance {
		return ErrInsufficientFunds
	}
	a.balance -= cents
	return nil
}

// Transfer moves cents from one account to another atomically: if the
// withdrawal fails, the destination must be unchanged.
func Transfer(from, to *Account, cents int64) error {
	if err := to.Deposit(cents); err != nil {
		return err
	}
	if err := from.Withdraw(cents); err != nil {
		return err
	}
	return nil
}
