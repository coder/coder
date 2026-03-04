const CHIME_PREFERENCE_KEY = "agents.chime-on-completion";

export function getChimeEnabled(): boolean {
	try {
		const stored = localStorage.getItem(CHIME_PREFERENCE_KEY);
		// Default to enabled when no preference has been saved.
		return stored === null ? true : stored === "true";
	} catch {
		return true;
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

function playChime(): void {
	try {
		if (!chimeAudio) {
			chimeAudio = new Audio("/chime.wav");
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

	playChime();
}
