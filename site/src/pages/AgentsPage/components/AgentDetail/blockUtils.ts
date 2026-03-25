import { asString } from "#/components/ai-elements/runtimeTypeUtils";
import type { RenderBlock } from "./types";

export const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

/**
 * Append a text or thinking block to a render block list, merging
 * with the previous block when the types match.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
): RenderBlock[] => {
	if (!text.trim()) {
		return blocks;
	}
	const nextBlocks = [...blocks];
	const last = nextBlocks[nextBlocks.length - 1];
	if (last && last.type === type) {
		nextBlocks[nextBlocks.length - 1] = {
			type,
			text: `${last.text}${text}`,
		};
		return nextBlocks;
	}
	nextBlocks.push({ type, text });
	return nextBlocks;
};
