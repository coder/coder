#!/usr/bin/env node
import { constants } from 'node:fs';
import { open } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join } from 'node:path';

try {
  const handle = await open(join(homedir(), '.local/state/coder-acp/job.json'), constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.size > 16384) throw new Error();
    const job = JSON.parse(await handle.readFile('utf8'));
    if (typeof job.status !== 'string' || !/^[a-z_-]{1,32}$/.test(job.status)) throw new Error();
    const id = typeof job.sessionId === 'string' && /^[0-9a-f-]{36}$/i.test(job.sessionId) ? job.sessionId : undefined;
    console.log((job.status === 'blocked' ? 'Initial task not submitted; inspect native sessions' : job.status) + (id ? ' (' + id + ')' : ''));
  } finally { await handle.close(); }
} catch (error) {
  console.log(error.code === 'ENOENT' ? 'Not dispatched' : 'Inspect job state');
}
