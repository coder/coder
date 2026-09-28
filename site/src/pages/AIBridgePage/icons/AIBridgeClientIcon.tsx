import { cn } from "cn";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

export const AIBridgeClientIcon = ({
	client,
	className,
}: {
	client: string | null;
	className?: string;
}) => {
	const iconClassName = "shrink-0";
	const fallbackIconClassName = "shrink-0 rounded-full bg-surface-tertiary";
	// This should be kept in sync with the client names in
	// the AI Bridge bridge.go file.
	// https://github.com/coder/aibridge/blob/main/bridge.go#L31-L32
	switch (client) {
		case "Coder Agents":
			return (
				<ExternalImage
					src="/icon/coder.svg"
					className={cn(iconClassName, className)}
				/>
			);
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
					src="/icon/openai-codex.svg"
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
		case "Xum":
			return (
				<ExternalImage
					src="/icon/xum.svg"
					className={cn(iconClassName, className)}
				/>
			);
		// Legacy label for interceptions recorded before the Mux -> Xum rename.
		case "Mux":
			return (
				<ExternalImage
					src="/icon/mux.svg"
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
		case "OpenCode":
			return (
				<ExternalImage
					src="/icon/opencode.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Charm Crush":
			return (
				<ExternalImage
					src="/icon/charm-crush.svg"
					className={cn(iconClassName, className)}
				/>
			);
		case "Junie":
			return (
				<ExternalImage
					src="/icon/junie.svg"
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
				<span aria-hidden className={cn(fallbackIconClassName, className)} />
			);
	}
};
