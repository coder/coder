UPDATE users SET chat_spend_limit_micros = 5000000
WHERE id = 'fc1511ef-4fcf-4a3b-98a1-8df64160e35a';

UPDATE groups SET chat_spend_limit_micros = 10000000
WHERE id = 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1';
