import type { AIBridgeInterception } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CircleQuestionMarkIcon } from "lucide-react";
import { cn } from "utils/cn";

export const AIBridgeProviderIcon = ({
	provider,
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
					className={cn(iconClassName, props.className)}
				/>
			);
		case "anthropic":
			return (
				<ExternalImage
					src="/icon/claude-device.svg"
					className={cn(iconClassName, props.className)}
				/>
			);
		default:
			return (
				<CircleQuestionMarkIcon
					className={cn(iconClassName, props.className)}
					{...props}
				/>
			);
	}
};
