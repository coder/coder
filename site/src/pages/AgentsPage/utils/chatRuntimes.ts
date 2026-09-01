import * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements";

export type ExternalChatRuntime = Exclude<TypesGen.ChatRuntime, "coder">;

interface ExternalChatRuntimeInfo {
	readonly label: string;
	readonly providerType: TypesGen.AIProviderType;
	readonly description: string;
}

// External runtimes inject their own provider credentials, so each one
// accepts model configs from a single provider family.
export const externalChatRuntimes: Record<
	ExternalChatRuntime,
	ExternalChatRuntimeInfo
> = {
	claude_code: {
		label: "Claude Code",
		providerType: "anthropic",
		description:
			"Delegate turns to Claude Code in a dedicated workspace. Anthropic models only; no attachments, plan mode, or MCP.",
	},
	codex: {
		label: "Codex",
		providerType: "openai",
		description:
			"Delegate turns to Codex in a dedicated workspace. OpenAI models only; no attachments, plan mode, or MCP.",
	},
};

export const isExternalChatRuntime = (
	runtime: TypesGen.ChatRuntime | undefined,
): runtime is ExternalChatRuntime =>
	runtime !== undefined && runtime !== "coder";

export const filterModelOptionsForRuntime = (
	options: readonly ModelSelectorOption[],
	runtime: ExternalChatRuntime,
): readonly ModelSelectorOption[] => {
	const { providerType } = externalChatRuntimes[runtime];
	return options.filter((option) => option.provider === providerType);
};

// Availability rows are per organization, so the composer only offers the
// runtimes enabled for the organization the chat will be created in.
export const availableExternalChatRuntimes = (
	availability: readonly TypesGen.ChatRuntimeAvailability[] | undefined,
	organizationId: string,
): readonly ExternalChatRuntime[] =>
	TypesGen.ChatRuntimes.filter(isExternalChatRuntime).filter((runtime) =>
		(availability ?? []).some(
			(row) =>
				row.runtime === runtime && row.organization_id === organizationId,
		),
	);
