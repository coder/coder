import { cn } from "cn";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

export const AIBridgeProviderIcon = ({
	provider,
	className,
}: {
	provider: string;
	className?: string;
}) => {
	const iconClassName = "shrink-0";
	const fallbackIconClassName = "shrink-0 rounded-full bg-surface-tertiary";
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
		case "openrouter":
			return (
				<ExternalImage
					src="/icon/openrouter.svg"
					className={cn(iconClassName, className)}
				/>
			);
		default:
			return (
				<span aria-hidden className={cn(fallbackIconClassName, className)} />
			);
	}
};
