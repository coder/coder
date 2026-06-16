import { CircleQuestionMarkIcon } from "lucide-react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { cn } from "#/utils/cn";

export const AIBridgeProviderIcon = ({
	provider,
	className,
	...props
}: {
	provider: string;
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
					src="/icon/anthropic.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "google":
			return (
				<ExternalImage
					src="/icon/google.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "azure":
			return (
				<ExternalImage
					src="/icon/azure.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "bedrock":
			return (
				<ExternalImage
					src="/icon/aws.svg"
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
		case "vercel":
			return (
				<ExternalImage
					src="/icon/vercel.svg"
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
