import { constants } from 'node:fs';
import { lstat, mkdir, open, rename, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, resolve } from 'node:path';

/** Hard archive limits, including JSON framing and metadata in the file budget. */
export const SESSION_LIMITS = Object.freeze({
  sessions: 100, events: 10000, eventBytes: 128 * 1024,
  journalBytes: 8 * 1024 * 1024, totalJournalBytes: 32 * 1024 * 1024,
  fileBytes: 40 * 1024 * 1024,
});
const fields = new Set(['sessionId', 'cwd', 'startedAt', 'runtimeId', 'cursor', 'historyStartCursor', 'runtimeStartCursor', 'events', 'fault', 'lastError']);
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const integer = value => Number.isSafeInteger(value) && value >= 0;
const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);

class StoreError extends Error {
  constructor(reason) {
    super('Session persistence: ' + reason + '. Inspect the private session file before restarting; nothing will be resumed automatically.');
    this.status = 503;
  }
}
function check(condition, reason = 'invalid or corrupt archive') {
  if (!condition) throw new StoreError(reason);
}

async function privateParent(file) {
  let current = '/';
  for (const part of dirname(file).split('/').filter(Boolean)) {
    current = join(current, part);
    let info;
    try { info = await lstat(current); }
    catch (error) {
      if (error.code !== 'ENOENT') throw error;
      try { await mkdir(current, { mode: 0o700 }); }
      catch (error) { if (error.code !== 'EEXIST') throw error; }
      info = await lstat(current);
    }
    check(info.isDirectory() && [0, process.getuid()].includes(info.uid) &&
      (!(info.mode & 0o022) || (info.mode & 0o1000)), 'unsafe directory or symlink');
  }
  const info = await lstat(dirname(file));
  check(info.uid === process.getuid() && !(info.mode & 0o077), 'directory must be owner-only (chmod 700)');
}
function checkFile(info) {
  check(info.isFile() && info.uid === process.getuid() && !(info.mode & 0o077) && info.nlink === 1,
    'file must be an owner-only regular file without links (chmod 600)');
}
const stamp = info => info && [info.dev, info.ino, info.size, info.mtimeMs, info.ctimeMs].join(':');

async function read(file) {
  let handle;
  try { handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK); }
  catch (error) { if (error.code === 'ENOENT') return { records: [], info: undefined }; throw error; }
  try {
    const info = await handle.stat();
    checkFile(info);
    check(info.size <= SESSION_LIMITS.fileBytes, 'archive exceeds the file budget');
    // Read at most the budget even if another process grows the file after stat.
    const bytes = Buffer.alloc(SESSION_LIMITS.fileBytes + 1);
    let size = 0;
    while (size < bytes.length) {
      const { bytesRead } = await handle.read(bytes, size, bytes.length - size, size);
      if (!bytesRead) break;
      size += bytesRead;
    }
    check(size <= SESSION_LIMITS.fileBytes, 'archive exceeds the file budget');
    check(stamp(info) === stamp(await handle.stat()), 'archive changed while reading');
    let value;
    try { value = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, size))); }
    catch { throw new StoreError('invalid or corrupt archive'); }
    validate(value);
    return { records: value.sessions, info };
  } finally { await handle.close(); }
}

function validate(value) {
  check(object(value) && value.version === 1 && Object.keys(value).length === 2 &&
    Array.isArray(value.sessions) && value.sessions.length <= SESSION_LIMITS.sessions);
  const ids = new Set();
  let totalBytes = 0;
  for (const s of value.sessions) {
    check(object(s) && Object.keys(s).every(key => fields.has(key)) && uuid(s.sessionId) && !ids.has(s.sessionId) &&
      typeof s.cwd === 'string' && isAbsolute(s.cwd) && integer(s.startedAt) && typeof s.fault === 'boolean' &&
      (s.runtimeId === undefined || (typeof s.runtimeId === 'string' && s.runtimeId.length > 0 && s.runtimeId.length < 200)) &&
      integer(s.cursor) && integer(s.historyStartCursor) && s.historyStartCursor <= s.cursor &&
      (s.runtimeStartCursor === undefined || (integer(s.runtimeStartCursor) && s.runtimeStartCursor <= s.cursor)) &&
      Array.isArray(s.events) && s.events.length <= SESSION_LIMITS.events);
    ids.add(s.sessionId);
    const { events, ...metadata } = s;
    check(Buffer.byteLength(JSON.stringify(metadata)) <= 32 * 1024, 'session metadata exceeds the budget');
    if (s.lastError !== undefined) check(object(s.lastError) && Object.keys(s.lastError).length === 2 &&
      typeof s.lastError.message === 'string' && s.lastError.message.length <= 512 && integer(s.lastError.at));
    let previous;
    let bytes = 0;
    for (const event of events) {
      check(object(event) && Object.keys(event).length === 3 && integer(event.seq) && event.seq > s.historyStartCursor &&
        event.seq <= s.cursor && event.seq === (previous ?? s.historyStartCursor) + 1 &&
        typeof event.type === 'string' && object(event.data));
      previous = event.seq;
      const size = Buffer.byteLength(JSON.stringify(event));
      // Broker cursors can have more digits than the native runtime's sequence.
      check(size <= SESSION_LIMITS.eventBytes + 64, 'event exceeds the budget');
      bytes += size;
    }
    check((previous ?? s.historyStartCursor) === s.cursor);
    check(bytes <= SESSION_LIMITS.journalBytes, 'session journal exceeds the budget');
    totalBytes += bytes;
  }
  check(totalBytes <= SESSION_LIMITS.totalJournalBytes, 'journals exceed the total budget');
}

/** Select archive data only, never runtime commands, credentials or launch settings. */
export function sessionRecord(s) {
  const events = s.events.filter(event => event.seq > s.historyStartCursor);
  return {
    sessionId: s.sessionId, cwd: s.cwd, startedAt: s.startedAt, runtimeId: s.runtimeId,
    cursor: s.cursor, historyStartCursor: Math.max(s.historyStartCursor, (events[0]?.seq ?? s.cursor + 1) - 1), fault: s.fault,
    runtimeStartCursor: s.runtimeStartCursor,
    lastError: s.lastError && { message: s.lastError.message, at: s.lastError.at },
    events,
  };
}

/** Open a bounded private archive. Writes are atomic, serialized and fsynced. */
export async function openSessionStore(path) {
  let file;
  let loaded;
  try {
    check(typeof path === 'string' && isAbsolute(path), 'sessionFile must be an absolute path');
    file = resolve(path);
    await privateParent(file);
    loaded = await read(file);
  } catch (error) { throw error instanceof StoreError ? error : new StoreError('cannot safely read archive'); }
  let info = loaded.info;
  let failure;
  let writing = Promise.resolve();
  // A fixed exclusive temporary path bounds crash leftovers. Only a caller that
  // has claimed exclusive descriptor ownership may discard an interrupted write.
  const temporary = file + '.tmp';
  async function guard() {
    await privateParent(file);
    let current;
    try { current = await lstat(file); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    if (current) checkFile(current);
    check(stamp(current) === stamp(info), 'archive was replaced or changed; refusing to overwrite it');
  }
  async function save(records) {
    if (failure) throw failure;
    let handle;
    let directory;
    let renamed = false;
    try {
      const value = { version: 1, sessions: records };
      validate(value);
      const text = JSON.stringify(value) + '\n';
      check(Buffer.byteLength(text) <= SESSION_LIMITS.fileBytes, 'archive exceeds the file budget');
      await guard();
      directory = await open(dirname(file), constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
      handle = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
      await handle.writeFile(text);
      await handle.sync();
      await guard();
      await rename(temporary, file);
      renamed = true;
      info = await handle.stat();
      await directory.sync();
    } catch (error) {
      failure = error instanceof StoreError ? error : new StoreError('cannot durably write archive; history was not acknowledged');
      throw failure;
    } finally {
      if (handle) {
        await handle.close();
        if (!renamed) await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw new StoreError('cannot clean up temporary archive'); });
      }
      await directory?.close();
    }
  }
  return {
    records: loaded.records,
    /** Requires exclusive ownership of the descriptor associated with this file. */
    discardInterruptedTemporary() {
      const pending = writing.then(async () => {
        if (failure) throw failure;
        let handle;
        let directory;
        try {
          await guard();
          try { handle = await open(temporary, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK); }
          catch (error) { if (error.code === 'ENOENT') return; throw error; }
          const interrupted = await handle.stat();
          checkFile(interrupted);
          check(interrupted.size <= SESSION_LIMITS.fileBytes, 'temporary archive exceeds the file budget');
          // Revalidate the primary snapshot after descriptor acquisition. Never
          // repair or replace a corrupt archive using an unacknowledged temporary.
          const current = await read(file);
          check(stamp(current.info) === stamp(info), 'archive changed before recovery');
          directory = await open(dirname(file), constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
          await guard();
          const temporaryInfo = await lstat(temporary);
          checkFile(temporaryInfo);
          check(stamp(temporaryInfo) === stamp(interrupted), 'temporary archive changed before recovery');
          await unlink(temporary);
          await directory.sync();
        } finally {
          try { await handle?.close(); } finally { await directory?.close(); }
        }
      }).catch(error => {
        failure ??= error instanceof StoreError ? error : new StoreError('cannot safely discard interrupted temporary archive');
        throw failure;
      });
      writing = pending.catch(() => {});
      return pending;
    },
    save(records) {
      const pending = writing.then(() => save(records)).catch(error => {
        failure ??= error instanceof StoreError ? error : new StoreError('cannot durably write archive; history was not acknowledged');
        throw failure;
      });
      writing = pending.catch(() => {});
      return pending;
    },
  };
}
