import type { AIBridgeInterception } from "api/typesGenerated";
import { DecorativeImage } from "components/ExternalImage";
import { CircleQuestionMarkIcon } from "lucide-react";
import { cn } from "utils/cn";

export const AIBridgeClientIcon = ({
	client,
	className,
	...props
}: {
	client: AIBridgeInterception["client"];
} & React.ComponentProps<"svg">) => {
	const iconClassName = "flex-shrink-0";
	// This should be kept in sync with the client names in
	// the AI Bridge bridge.go file.
	// https://github.com/coder/aibridge/blob/main/bridge.go#L31-L32
	switch (client) {
		case "Claude Code":
			return (
				<DecorativeImage
					src="/icon/claude.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Codex":
			return (
				<DecorativeImage
					src="/icon/openai-codex.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Kilo Code":
			return (
				<DecorativeImage
					src="/icon/kilo-code.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Roo Code":
			return (
				<DecorativeImage
					src="/icon/roo-code.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Mux":
			return (
				<DecorativeImage
					src="/icon/mux.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Zed":
			return (
				<DecorativeImage
					src="/icon/zed.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Cursor":
			return (
				<DecorativeImage
					src="/icon/cursor.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "GitHub Copilot (VS Code)":
		case "GitHub Copilot (CLI)":
			return (
				<DecorativeImage
					src="/icon/github-copilot.svg"
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
