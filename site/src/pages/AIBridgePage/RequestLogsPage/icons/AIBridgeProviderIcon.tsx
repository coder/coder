import type { AIBridgeInterception } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CircleQuestionMarkIcon } from "lucide-react";
import { cn } from "utils/cn";

export const AIBridgeProviderIcon = ({
	provider,
	className,
	...props
}: {
	provider: AIBridgeInterception["provider"];
} & React.ComponentProps<"svg">) => {
	const iconClassName = "flex-shrink-0";
	switch (provider) {
		case "openai":
			return (
				<ExternalImage
					src="/icon/openai.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "anthropic":
			return (
				<ExternalImage
					src="/icon/claude.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "copilot":
			return (
				<ExternalImage
					src="/icon/github.svg"
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
