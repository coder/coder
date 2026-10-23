import { constants } from 'node:fs';
import { open } from 'node:fs/promises';

/** Read a private descriptor and reject accidental remote control endpoints. */
export async function connect(file) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  } catch (error) {
    if (error.code === 'ENOENT') throw new Error('Bridge descriptor missing: ' + file + '. Start the broker explicitly with coder-acp serve --detach; clients never start it.');
    throw new Error('Cannot open private bridge descriptor: ' + file + '. Check ownership and permissions; symlinks are not supported.');
  }
  let descriptor;
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0) throw new Error('Bridge descriptor must be an owner-only regular file, not accessible to other users (chmod 600)');
    try { descriptor = JSON.parse(await handle.readFile('utf8')); }
    catch { throw new Error('Invalid bridge descriptor JSON; inspect or replace it only after confirming the broker is stopped.'); }
  } finally {
    await handle.close();
  }
  // JSON and URL parser diagnostics can contain the private token; never echo them.
  let url;
  try { url = new URL(descriptor?.url); } catch { throw new Error('Invalid loopback bridge descriptor'); }
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port || url.origin !== descriptor.url || typeof descriptor.token !== 'string' || !/^[a-f0-9]{64}$/.test(descriptor.token)) throw new Error('Invalid loopback bridge descriptor');
  return createClient(descriptor);
}

/** Build the authenticated broker client; never retry mutating requests. */
export function createClient({ url, token }) {
  return {
    async request(method, path, body, signal) {
      let response;
      try {
        response = await fetch(url + path, {
          method,
          headers: { Authorization: 'Bearer ' + token, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
          body: body === undefined ? undefined : JSON.stringify(body),
          signal: AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(path === '/v1/sessions' && method === 'POST' ? 65000 : 25000)]),
          redirect: 'error',
        });
      } catch {
        if (signal?.aborted) throw new DOMException('Bridge request aborted; native work may still be running.', 'AbortError');
        throw new Error('Bridge broker offline or request interrupted; descriptor may be stale. Work state is unknown: inspect the broker log and claude agents before retrying. Nothing was restarted.');
      }
      if (response.status === 401 || response.status === 403) {
        const error = new Error('Bridge descriptor is stale or authentication was rejected. Check that clients and broker use the same state file; nothing was restarted.');
        error.status = response.status;
        throw error;
      }
      let value;
      try { value = await response.json(); }
      catch { throw new Error('Invalid broker response; descriptor may be stale. Work state is unknown; inspect before retrying.'); }
      if (!response.ok) {
        const error = new Error(typeof value?.error === 'string' ? value.error.replaceAll(token, '[redacted]') : 'Broker request failed');
        error.status = response.status;
        throw error;
      }
      return value;
    },
  };
}
