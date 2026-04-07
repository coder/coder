const CHIME_PREFERENCE_KEY = "agents.chime-on-completion";
const KYLEOSOPHY_PREFERENCE_KEY = "agents.kyleosophy";

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
 * Whether Kyleosophy mode is active. Force-enabled on
 * dev.coder.com because the people deserve Kyle.
 */
export function getKylesophyEnabled(): boolean {
	if (isKylesophyForced()) {
		return true;
	}
	try {
		const stored = localStorage.getItem(KYLEOSOPHY_PREFERENCE_KEY);
		return stored === null ? false : stored === "true";
	} catch {
		return false;
	}
}

/**
 * Whether the current deployment force-enables Kyleosophy,
 * bypassing the user preference.
 */
export function isKylesophyForced(): boolean {
	try {
		return globalThis.location?.hostname === "dev.coder.com";
	} catch {
		return false;
	}
}

export function setKylesophyEnabled(enabled: boolean): void {
	try {
		localStorage.setItem(KYLEOSOPHY_PREFERENCE_KEY, String(enabled));
	} catch {
		// Silently ignore storage errors (e.g. private browsing
		// quota exceeded).
	}
}

/**
 * Alternative completion sounds for Kyleosophy mode. All are
 * shipped as static assets alongside chime.mp3.
 */
export const KYLEOSOPHY_SOUNDS: readonly string[] = [
	"/chime_1.mp3", // absolutely massive
	"/chime_2.mp3", // dope
	"/chime_3.mp3", // great
	"/chime_4.mp3", // oh god
	"/chime_5.mp3", // okay
	"/chime_6.mp3", // open up a pr
	"/chime_7.mp3", // sweet
	"/chime_8.mp3", // yep
];

/**
 * Play a completion sound. When Kyleosophy is enabled a random
 * voice clip is selected; otherwise the default bell chime is
 * used. The Audio element is cached and reused when the sound
 * URL hasn't changed between calls.
 */
let chimeAudio: HTMLAudioElement | null = null;
let lastSoundUrl: string | null = null;

/** @internal Reset cached Audio state between tests. */
export function _resetForTesting(): void {
	chimeAudio = null;
	lastSoundUrl = null;
}

function playChimeAudio(soundUrl = "/chime.mp3"): void {
	try {
		if (!chimeAudio || soundUrl !== lastSoundUrl) {
			chimeAudio?.pause();
			chimeAudio = new Audio(soundUrl);
			chimeAudio.volume = 0.5;
			lastSoundUrl = soundUrl;
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
function playChime(chatID: string, soundUrl?: string): void {
	if (typeof navigator === "undefined" || !navigator.locks) {
		playChimeAudio(soundUrl);
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

			playChimeAudio(soundUrl);

			// Hold the lock briefly so that tabs receiving the
			// WebSocket event a bit later will see the lock as
			// held and skip.
			await new Promise((resolve) => setTimeout(resolve, LOCK_HOLD_MS));
		},
	);
}

/**
 * Check whether a chat status transition should trigger a chime
 * and play it if so. The chime fires on these transitions:
 *
 *   running → waiting   (normal completion via per-chat WS)
 *   running → pending   (normal completion via per-chat WS)
 *   pending → waiting   (watchChats WS skipped "running")
 *
 * Note that "pending" appears as both a source and a target:
 * it is an active state when the agent is queued, and a resting
 * state after the agent finishes. The chime is suppressed when
 * the chat is currently visible to the user.
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

	const soundUrl = getKylesophyEnabled()
		? KYLEOSOPHY_SOUNDS[Math.floor(Math.random() * KYLEOSOPHY_SOUNDS.length)]
		: undefined;

	playChime(chatID, soundUrl);
}
