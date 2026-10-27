import { useRef } from "react";

export const ARTIFACT_PROMPT_TYPE = "coder:artifact-prompt";
const ARTIFACT_PROMPT_MAX_LENGTH = 4000;
/** A second artifact message within this window waits in the chat input. */
const ARTIFACT_SEND_INTERVAL_MS = 5000;

type ArtifactFrameState = {
	frame: HTMLIFrameElement | null;
	/** True after the frame loaded a second document. */
	navigated: boolean;
	activeElement: Element | null;
	userActivationActive: boolean;
};

/**
 * Returns the prompt text of a message from an HTML artifact frame, or
 * undefined when the message fails a check. The frame has an opaque origin,
 * so the message source identifies it. Focus on the frame and an active user
 * activation mean that the user just clicked in it.
 */
export const acceptArtifactMessage = (
	event: Pick<MessageEvent, "source" | "data">,
	state: ArtifactFrameState,
): string | undefined => {
	const { frame } = state;
	if (!frame || state.navigated || event.source !== frame.contentWindow) {
		return undefined;
	}
	if (state.activeElement !== frame || !state.userActivationActive) {
		return undefined;
	}
	const data: unknown = event.data;
	if (typeof data !== "object" || data === null) {
		return undefined;
	}
	if (!("type" in data) || data.type !== ARTIFACT_PROMPT_TYPE) {
		return undefined;
	}
	if (!("text" in data) || typeof data.text !== "string") {
		return undefined;
	}
	if (data.text.length === 0 || data.text.length > ARTIFACT_PROMPT_MAX_LENGTH) {
		return undefined;
	}
	return data.text;
};

/**
 * Decides what an accepted artifact prompt does in a chat. It sends at once,
 * unless the previous artifact prompt was sent less than
 * ARTIFACT_SEND_INTERVAL_MS ago. A slash command never sends, because it
 * runs a built-in command such as clearing the chat context.
 */
export const artifactPromptAction = (
	text: string,
	lastSentAt: number | undefined,
	now: number,
): "send" | "input" => {
	if (text.trimStart().startsWith("/")) {
		return "input";
	}
	if (
		lastSentAt !== undefined &&
		now - lastSentAt < ARTIFACT_SEND_INTERVAL_MS
	) {
		return "input";
	}
	return "send";
};

type ArtifactPromptHandlerOptions = {
	/** False while the chat cannot take a new turn. */
	canSend: boolean;
	send: (text: string) => void;
	insert: (text: string) => void;
};

/**
 * Returns the chat page handler for accepted artifact prompts. It applies
 * artifactPromptAction and remembers when it last sent a prompt.
 */
export const useArtifactPromptHandler = ({
	canSend,
	send,
	insert,
}: ArtifactPromptHandlerOptions) => {
	const lastSentAtRef = useRef<number | undefined>(undefined);
	return (text: string) => {
		const now = Date.now();
		if (
			!canSend ||
			artifactPromptAction(text, lastSentAtRef.current, now) === "input"
		) {
			insert(text);
			return;
		}
		lastSentAtRef.current = now;
		send(text);
	};
};
