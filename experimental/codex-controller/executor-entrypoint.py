"""Read the restricted connection credential from stdin, then replace this process."""
import json
import os
import re
import sys

config = json.loads(sys.stdin.readline())
if not re.fullmatch(r'https://api\.openai\.com/v1/agents/api/connect(?:/[A-Za-z0-9_-]{1,200}){1,8}', config['remote_url']):
    raise SystemExit('Invalid remote URL')
if not re.fullmatch(r'[A-Za-z0-9_-]{1,200}', config['environment_id']):
    raise SystemExit('Invalid environment ID')
key = config['executor_key']
if not isinstance(key, str) or len(key) < 8 or re.search(r'\s', key):
    raise SystemExit('Invalid executor credential')
os.makedirs('/tmp/codex-home', mode=0o700, exist_ok=True)
os.chdir('/home/coder/demo')
os.execve('/usr/local/bin/codex', ['codex', 'exec-server', '--remote', config['remote_url'], '--environment-id', config['environment_id']], {
    'HOME': '/tmp', 'CODEX_HOME': '/tmp/codex-home', 'PATH': '/usr/local/bin:/usr/bin:/bin',
    'CODEX_API_KEY': key, 'RUST_LOG': 'info',
})
