# webhook

Sends order events to the payments webhook. `config.py` loads settings from
the environment and from `.env`. Run `python3 client.py --dry-run` to see the
request it would send, and the tests with
`python3 -m unittest discover -s tests -t .`.
