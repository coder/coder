const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const OPENCODE_ID = /^ses_[a-zA-Z0-9]{1,128}$/;

/** Native identities accepted at the authenticated broker and ACP boundaries. */
export function nativeIdentity(id) {
  if (typeof id !== 'string') return;
  if (UUID.test(id)) return { harness: 'claude', nativeSessionId: id };
  if (id.startsWith('codex:') && UUID.test(id.slice(6))) return { harness: 'codex', nativeSessionId: id.slice(6) };
  if (id.startsWith('opencode:') && OPENCODE_ID.test(id.slice(9))) return { harness: 'opencode', nativeSessionId: id.slice(9) };
}

/** Display identities advertised independently of each native model provider. */
export const HARNESS_INFO = Object.freeze({
  claude: { id: 'claude', name: 'claude-acp-bridge', title: 'Claude Code' },
  codex: { id: 'codex', name: 'codex', title: 'Codex' },
  opencode: { id: 'opencode', name: 'opencode', title: 'OpenCode' },
});
