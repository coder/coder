import type { AIBridgeInterception } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
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
	switch (client) {
		case "Claude Code":
			return (
				<ExternalImage
					src="/icon/claude.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Codex":
			return (
				<ExternalImage
					src="/icon/openai.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Kilo Code":
			return (
				<ExternalImage
					src="/icon/kilo-code.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Roo Code":
			return (
				<ExternalImage
					src="/icon/roo-code.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Zed":
			return (
				<ExternalImage
					src="/icon/zed.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Cursor":
			return (
				<ExternalImage
					src="/icon/cursor.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "GitHub Copilot (VS Code)":
		case "GitHub Copilot (CLI)":
			return (
				<ExternalImage
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
