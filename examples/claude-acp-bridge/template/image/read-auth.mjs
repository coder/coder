#!/usr/bin/env node
import { constants } from 'node:fs';
import { open } from 'node:fs/promises';

// Only Claude's apiKeyHelper reads stdout. Never invoke this in startup logs.
try {
  const handle = await open('/run/secrets/claude-auth.json', constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.size > 1024 * 1024) throw new Error();
    const { ANTHROPIC_API_KEY: key } = JSON.parse(await handle.readFile('utf8'));
    if (typeof key !== 'string' || !key || /\s/.test(key)) throw new Error();
    process.stdout.write(key + '\n');
  } finally { await handle.close(); }
} catch {
  console.error('Cannot read the private Claude API key.');
  process.exitCode = 1;
}
