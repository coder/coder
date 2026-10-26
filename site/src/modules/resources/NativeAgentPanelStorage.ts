import type { SessionInfo } from "@agentclientprotocol/sdk";
import * as Yup from "yup";

type NativeAgentPanelState = {
	cwd: string;
	harness: string;
	prompt: string;
	selectedSession?: Pick<SessionInfo, "sessionId" | "cwd">;
	collapsedSessionID?: string;
};

const panelStateSchema: Yup.ObjectSchema<NativeAgentPanelState> = Yup.object({
	cwd: Yup.string().max(4_096).defined(),
	harness: Yup.string().max(128).defined(),
	prompt: Yup.string().max(32_000).defined(),
	selectedSession: Yup.object({
		sessionId: Yup.string().max(1_024).required(),
		cwd: Yup.string().max(4_096).required(),
	})
		.noUnknown()
		.default(undefined)
		.optional(),
	collapsedSessionID: Yup.string().max(1_024).optional(),
})
	.noUnknown()
	.strict();

const MAX_STORED_LENGTH = 256_000;

/** Agent names are case-insensitively unique per workspace and survive rebuilds. */
export function nativeAgentPanelStorageKey(
	userID: string,
	workspaceID: string,
	agentName: string,
): string {
	return `native-agent-panel:${JSON.stringify([userID, workspaceID, agentName.toLowerCase()])}`;
}

/** Restores tab-local UI state only; transcripts and adapter metadata are never stored. */
export function readNativeAgentPanelState(
	key: string,
): NativeAgentPanelState | undefined {
	try {
		const raw = sessionStorage.getItem(key);
		if (!raw || raw.length > MAX_STORED_LENGTH) return undefined;
		return panelStateSchema.validateSync(JSON.parse(raw));
	} catch {
		return undefined;
	}
}

/** Storage denial or oversized input must not prevent editing the live form. */
export function writeNativeAgentPanelState(
	key: string,
	state: NativeAgentPanelState,
): void {
	try {
		if (!panelStateSchema.isValidSync(state)) {
			sessionStorage.removeItem(key);
			return;
		}
		sessionStorage.setItem(key, JSON.stringify(state));
	} catch {
		// Persistence is best-effort; never reset the in-memory draft on failure.
	}
}
