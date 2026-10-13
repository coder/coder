# slowtests

Integration tests for the retry helper. They are slow because each one waits
for real back-off timers. Run them with
`python3 -m unittest discover -s tests -t . -v`.
