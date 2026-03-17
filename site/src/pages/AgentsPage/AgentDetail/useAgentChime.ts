const CHIME_PREFERENCE_KEY = "agents.chime-on-completion";

export function getChimeEnabled(): boolean {
	try {
		const stored = localStorage.getItem(CHIME_PREFERENCE_KEY);
		// Default to disabled when no preference has been saved.
		return stored === null ? false : stored === "true";
	} catch {
		return false;
	}
}

export function setChimeEnabled(enabled: boolean): void {
	try {
		localStorage.setItem(CHIME_PREFERENCE_KEY, String(enabled));
	} catch {
		// Silently ignore storage errors (e.g. private browsing
		// quota exceeded).
	}
}

/**
 * Play the completion chime audio file. The file is a short,
 * warm two-tone bell sound shipped as a static asset.
 *
 * A single Audio element is reused across calls so the browser
 * only fetches the file once.
 */
let chimeAudio: HTMLAudioElement | null = null;

function playChimeAudio(): void {
	try {
		if (!chimeAudio) {
			chimeAudio = new Audio("/chime.mp3");
			chimeAudio.volume = 0.5;
		}
		// Reset to the start in case a previous play hasn't
		// finished yet.
		chimeAudio.currentTime = 0;
		void chimeAudio.play();
	} catch {
		// Silently ignore playback errors (e.g. autoplay policy
		// blocks, missing file, etc.).
	}
}

// -- Cross-tab chime deduplication via Web Locks API ----------
//
// When multiple tabs are open on /agents, every tab receives the
// same WebSocket status transitions and would independently
// decide to play the chime. We use navigator.locks to acquire a
// short-lived, per-chatID lock. Only the tab that successfully
// acquires the lock plays the sound. The lock is held for a
// short duration to prevent other tabs from acquiring it for the
// same event.
//
// Falls back to always playing (original single-tab behavior)
// when the Web Locks API is unavailable.

/**
 * How long to hold the lock after playing the chime (ms). This
 * prevents other tabs whose WebSocket event arrives slightly
 * later from also acquiring the lock for the same transition.
 */
export const LOCK_HOLD_MS = 2000;

/**
 * Coordinate across tabs so that only one tab plays the chime
 * for a given chatID. Uses navigator.locks.request() with
 * ifAvailable: true — the first tab to acquire the lock plays,
 * all others silently skip. The lock is held for LOCK_HOLD_MS
 * to cover the window in which other tabs receive the same
 * WebSocket event.
 *
 * Falls back to playing immediately when the Web Locks API is
 * not available (preserving the original single-tab behavior).
 */
function playChime(chatID: string): void {
	if (typeof navigator === "undefined" || !navigator.locks) {
		playChimeAudio();
		return;
	}

	const lockName = `coder-agent-chime:${chatID}`;

	void navigator.locks.request(
		lockName,
		{ ifAvailable: true },
		async (lock) => {
			if (!lock) {
				// Another tab already holds the lock for this
				// chatID — skip playback.
				return;
			}

			playChimeAudio();

			// Hold the lock briefly so that tabs receiving the
			// WebSocket event a bit later will see the lock as
			// held and skip.
			await new Promise((resolve) => setTimeout(resolve, LOCK_HOLD_MS));
		},
	);
}

/**
 * Check whether a chat status transition should trigger a chime
 * and play it if so. A chime fires when a chat reaches a
 * terminal state ("waiting" or "error") from a non-terminal
 * state, meaning the agent just finished work. The previous
 * status may be "running" (seen via the per-chat WebSocket) or
 * "pending" (when only the watchChats WebSocket is active and
 * the intermediate "running" status was never pushed to the
 * chat list). The chime is suppressed when the chat is
 * currently visible to the user.
 */
export function maybePlayChime(
	prevStatus: string | undefined,
	nextStatus: string,
	chatID: string,
	activeChatID: string | undefined,
): void {
	if (prevStatus === nextStatus) {
		return;
	}

	// Terminal states that indicate the agent finished.
	const isTerminal = nextStatus === "waiting" || nextStatus === "pending";
	if (!isTerminal) {
		return;
	}

	// Only chime when transitioning from a non-terminal state.
	// "running" is the expected previous state, but "pending" can
	// appear when the watchChats WebSocket skips the intermediate
	// "running" status (it only publishes the final state change).
	const wasActive = prevStatus === "running" || prevStatus === "pending";
	if (!wasActive) {
		return;
	}

	// Skip when the user is looking at this exact chat.
	const isViewingThisChat = !document.hidden && chatID === activeChatID;
	if (isViewingThisChat) {
		return;
	}

	if (!getChimeEnabled()) {
		return;
	}

	playChime(chatID);
}
