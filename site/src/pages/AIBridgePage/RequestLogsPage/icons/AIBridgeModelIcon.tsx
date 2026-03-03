import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CircleQuestionMarkIcon } from "lucide-react";
import { cn } from "utils/cn";

// Infers the model family from a model name string.
// See official model naming docs:
// - Anthropic: https://docs.anthropic.com/en/docs/about-claude/models/all-models
// - OpenAI: https://platform.openai.com/docs/models
function inferModelFamily(model: string): string {
	const modelFamily = model.toLowerCase();
	// Anthropic model families
	if (
		modelFamily.includes("claude") ||
		modelFamily.includes("opus") ||
		modelFamily.includes("sonnet") ||
		modelFamily.includes("haiku")
	) {
		return "claude";
	}
	// OpenAI model families (gpt-*, o1/o3/o4-*, codex, whisper)
	if (
		modelFamily.includes("gpt") ||
		modelFamily.startsWith("o1") ||
		modelFamily.startsWith("o3") ||
		modelFamily.startsWith("o4") ||
		modelFamily.includes("codex") ||
		modelFamily.includes("whisper")
	) {
		return "openai";
	}
	return "unknown";
}

export const AIBridgeModelIcon = ({
	model,
	className,
	...props
}: {
	model: string;
} & React.ComponentProps<"svg">) => {
	const iconClassName = "flex-shrink-0";
	const family = inferModelFamily(model);
	switch (family) {
		case "claude":
			return (
				<ExternalImage
					src="/icon/claude.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "openai":
			return (
				<ExternalImage
					src="/icon/openai.svg"
					className={cn(iconClassName, className)}
				/>
			);
		default:
			return (
				<CircleQuestionMarkIcon
					className={cn(iconClassName, className)}
					{...props}
				/>
			);
	}
};
