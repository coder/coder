import {
	AnthropicInlineImageCapBytes,
	MaxChatFileSizeBytes,
} from "#/api/typesGenerated";
import { formatProviderLabel } from "./modelOptions";

// Budgets sit below the wire limits to leave room for encoder framing
// overhead, so a file at exactly the budget is still under the
// server's hard cap.
const FRAMING_MARGIN_BYTES = 16 * 1024;
const DEFAULT_IMAGE_BUDGET_BYTES = MaxChatFileSizeBytes - FRAMING_MARGIN_BYTES;
const ANTHROPIC_IMAGE_BUDGET_BYTES =
	AnthropicInlineImageCapBytes - FRAMING_MARGIN_BYTES;

// Must mirror chatprovider.InlineImageCapBytes on the server.
const ANTHROPIC_STRICT_BUDGET_PROVIDERS: ReadonlySet<string> = new Set([
	"anthropic",
	"bedrock",
]);

// Inputs are normalized to match chatprovider.NormalizeProvider on
// the server, so callers don't have to pre-normalize.
export function imageBudgetForProvider(provider: string | undefined): number {
	const normalized = provider?.trim().toLowerCase();
	if (normalized && ANTHROPIC_STRICT_BUDGET_PROVIDERS.has(normalized)) {
		return ANTHROPIC_IMAGE_BUDGET_BYTES;
	}
	return DEFAULT_IMAGE_BUDGET_BYTES;
}

export function formatMiB(bytes: number): string {
	return (bytes / 1024 / 1024).toFixed(1);
}

export function providerBudgetError(
	provider: string | undefined,
	actualBytes: number,
	budgetBytes: number,
): string {
	const label = provider ? formatProviderLabel(provider) : "this provider";
	return `Image too large for ${label} (${formatMiB(actualBytes)} MiB). Inline images must be under ${formatMiB(budgetBytes)} MiB on this provider.`;
}

export function imageNeedsResize(file: File, budget: number): boolean {
	return file.type.startsWith("image/") && file.size > budget;
}
