#!/usr/bin/env bash
# Withdraw first; deposit only once the source has actually paid.
cd /app && python3 - <<'PY'
import re
p='ledger.go'; s=open(p).read()
s=s.replace('''	if err := to.Deposit(cents); err != nil {
		return err
	}
	if err := from.Withdraw(cents); err != nil {
		return err
	}
	return nil''','''	if err := from.Withdraw(cents); err != nil {
		return err
	}
	if err := to.Deposit(cents); err != nil {
		return err
	}
	return nil''')
open(p,'w').write(s)
PY
