import { asString } from "components/ai-elements/runtimeTypeUtils";
import type { RenderBlock } from "./types";

const createBlock = (
	type: "response" | "thinking",
	text: string,
	title?: string,
): RenderBlock =>
	type === "thinking" ? { type, text, title } : { type, text };

export const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

export const mergeThinkingTitles = (
	currentTitle: string | undefined,
	nextTitle: string | undefined,
): { shouldMerge: boolean; title: string | undefined } => {
	if (!currentTitle && !nextTitle) {
		return { shouldMerge: true, title: undefined };
	}
	if (!currentTitle) {
		return { shouldMerge: true, title: nextTitle };
	}
	if (!nextTitle) {
		return { shouldMerge: true, title: currentTitle };
	}
	if (currentTitle === nextTitle) {
		return { shouldMerge: true, title: currentTitle };
	}
	if (nextTitle.startsWith(currentTitle)) {
		return { shouldMerge: true, title: nextTitle };
	}
	if (currentTitle.startsWith(nextTitle)) {
		return { shouldMerge: true, title: currentTitle };
	}
	return { shouldMerge: false, title: nextTitle };
};

/**
 * Append a text or thinking block to a render block list, merging
 * with the previous block when the types match (and thinking titles
 * are compatible).
 *
 * @param joinText Controls how existing and new text are concatenated
 *   when merging into an existing block. Callers that process
 *   complete message blocks typically join with a newline, while
 *   streaming callers concatenate directly.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
	title?: string,
	joinText: (current: string, next: string) => string = (a, b) => `${a}${b}`,
): RenderBlock[] => {
	if (!text.trim()) {
		return blocks;
	}
	const nextBlocks = [...blocks];
	const last = nextBlocks[nextBlocks.length - 1];
	if (last && last.type === type) {
		const shouldMerge =
			type === "response" ||
			(type === "thinking" &&
				last.type === "thinking" &&
				mergeThinkingTitles(last.title, title).shouldMerge);
		if (shouldMerge) {
			const mergedTitle =
				type === "thinking" && last.type === "thinking"
					? mergeThinkingTitles(last.title, title).title
					: undefined;
			nextBlocks[nextBlocks.length - 1] = createBlock(
				type,
				joinText(last.text, text),
				mergedTitle,
			);
			return nextBlocks;
		}
	}
	nextBlocks.push(createBlock(type, text, title));
	return nextBlocks;
};
