import { useEffect, useRef, useSyncExternalStore } from "react";
import { toast } from "sonner";

/**
 * Easter egg. Typing "iddqd" anywhere in the Agents UI (including while the
 * composer has focus) swaps the Thinking indicator for a small slot machine;
 * typing it again turns it off. The toggle is persisted in localStorage so it
 * survives reloads. This is intentional and has no effect outside the
 * indicator's visuals.
 */
export const slotMachineStorageKey = "agents.slot-machine";
export const slotMachineKeySequence = "iddqd";

/**
 * Reports whether a keydown event contributes a printable character that the
 * sequence buffer should record. Ctrl, Meta, and Alt chords are skipped.
 * Shift is allowed because it still produces a printable key.
 */
export function isPrintableKeyEvent(event: {
	key: string;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
}): boolean {
	if (event.ctrlKey || event.metaKey || event.altKey) {
		return false;
	}
	// Named keys such as "Enter" or "ArrowLeft" are longer than one code point.
	return [...event.key].length === 1;
}

/**
 * Appends a key to the buffer, lowercased, and keeps only the trailing
 * `maxLength` characters.
 */
export function appendToKeyBuffer(
	buffer: string,
	key: string,
	maxLength: number,
): string {
	return (buffer + key.toLowerCase()).slice(-maxLength);
}

/** Reports whether the buffer ends with the sequence. */
export function matchesKeySequence(buffer: string, sequence: string): boolean {
	return sequence.length > 0 && buffer.endsWith(sequence);
}

const listeners = new Set<() => void>();

function readSlotMachineEnabled(): boolean {
	try {
		return localStorage.getItem(slotMachineStorageKey) === "true";
	} catch {
		return false;
	}
}

function writeSlotMachineEnabled(enabled: boolean): void {
	try {
		localStorage.setItem(slotMachineStorageKey, String(enabled));
	} catch {
		// Storage can be unavailable (private mode, quota); the toggle then
		// only lasts for the current page.
	}
	for (const listener of listeners) {
		listener();
	}
}

function subscribeSlotMachineEnabled(listener: () => void): () => void {
	listeners.add(listener);
	window.addEventListener("storage", listener);
	return () => {
		listeners.delete(listener);
		window.removeEventListener("storage", listener);
	};
}

/** Whether the slot machine indicator is currently enabled. */
export function useSlotMachineEnabled(): boolean {
	return useSyncExternalStore(
		subscribeSlotMachineEnabled,
		readSlotMachineEnabled,
	);
}

/**
 * Installs a document-level keydown listener that watches for the toggle
 * sequence. Events are only observed, never prevented or stopped, so typed
 * characters still reach whichever element has focus.
 */
export function useSlotMachineEasterEggListener(): void {
	const bufferRef = useRef("");

	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			if (!isPrintableKeyEvent(event)) {
				return;
			}
			bufferRef.current = appendToKeyBuffer(
				bufferRef.current,
				event.key,
				slotMachineKeySequence.length,
			);
			if (!matchesKeySequence(bufferRef.current, slotMachineKeySequence)) {
				return;
			}
			bufferRef.current = "";
			const enabled = !readSlotMachineEnabled();
			writeSlotMachineEnabled(enabled);
			toast(enabled ? "Slot machine mode on" : "Slot machine mode off");
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, []);
}

const reducedMotionQuery = "(prefers-reduced-motion: reduce)";

function subscribeReducedMotion(listener: () => void): () => void {
	const media = window.matchMedia(reducedMotionQuery);
	media.addEventListener("change", listener);
	return () => media.removeEventListener("change", listener);
}

function readReducedMotion(): boolean {
	return window.matchMedia(reducedMotionQuery).matches;
}

/** Whether the user has asked for reduced motion. */
export function usePrefersReducedMotion(): boolean {
	return useSyncExternalStore(subscribeReducedMotion, readReducedMotion);
}
