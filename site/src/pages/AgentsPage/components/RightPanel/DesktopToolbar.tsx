import {
	ExternalLinkIcon,
	HandIcon,
	MaximizeIcon,
	MinimizeIcon,
	MousePointer2Icon,
} from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";

type ScaleMode = "native" | "fit";

interface DesktopToolbarProps {
	agent?: TypesGen.WorkspaceAgent;
	workspace?: TypesGen.Workspace;
	scaleMode: ScaleMode;
	onScaleModeChange: (mode: ScaleMode) => void;
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
	onPopOut?: () => void;
	isPoppedOut?: boolean;
}

/**
 * Compact icon button for a single workspace app in the desktop toolbar.
 */
const ToolbarAppIcon: FC<{
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	workspace: TypesGen.Workspace;
}> = ({ app, agent, workspace }) => {
	const link = useAppLink(app, { agent, workspace });

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<a
					href={link.href}
					onClick={link.onClick}
					target="_blank"
					rel="noreferrer"
					className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary"
					aria-label={link.label}
				>
					{app.icon ? (
						<ExternalImage src={app.icon} alt="" className="size-4" />
					) : (
						<span className="text-xs font-medium">
							{(app.display_name || app.slug).charAt(0).toUpperCase()}
						</span>
					)}
				</a>
			</TooltipTrigger>
			<TooltipContent>{link.label}</TooltipContent>
		</Tooltip>
	);
};

export const DesktopToolbar: FC<DesktopToolbarProps> = ({
	agent,
	workspace,
	scaleMode,
	onScaleModeChange,
	isControlling,
	onTakeControl,
	onReleaseControl,
	onPopOut,
	isPoppedOut,
}) => {
	// Filter to visible apps only.
	const apps = agent?.apps.filter((app) => !app.hidden) ?? [];

	return (
		<div
			className={cn(
				"group/toolbar absolute top-0 right-0 left-0 z-20 flex items-center justify-between px-2 transition-all duration-200",
				isControlling
					? "h-1 opacity-0 hover:h-9 hover:opacity-100 hover:bg-surface-primary/90 hover:backdrop-blur-sm"
					: "h-9 bg-surface-primary/90 backdrop-blur-sm",
			)}
			role="toolbar"
			aria-label="Desktop controls"
		>
			{/* Left: App icons */}
			<div className="flex items-center gap-0.5">
				{agent &&
					workspace &&
					apps.map((app) => (
						<ToolbarAppIcon
							key={app.slug}
							app={app}
							agent={agent}
							workspace={workspace}
						/>
					))}
			</div>

			{/* Right: Controls */}
			<div className="flex items-center gap-1">
				{/* Zoom toggle */}
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							onClick={() =>
								onScaleModeChange(scaleMode === "native" ? "fit" : "native")
							}
							aria-label={
								scaleMode === "native"
									? "Switch to fit-to-window (Ctrl+0)"
									: "Switch to 100% zoom (Ctrl+1)"
							}
							className="h-7 w-7 text-content-secondary hover:text-content-primary"
						>
							{scaleMode === "native" ? (
								<MinimizeIcon className="size-3.5" />
							) : (
								<MaximizeIcon className="size-3.5" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						{scaleMode === "native"
							? "Fit to window (Ctrl+0)"
							: "100% zoom (Ctrl+1)"}
					</TooltipContent>
				</Tooltip>

				{/* Pop-out button */}
				{onPopOut && !isPoppedOut && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="subtle"
								size="icon"
								onClick={onPopOut}
								aria-label="Open desktop in new window"
								className="h-7 w-7 text-content-secondary hover:text-content-primary"
							>
								<ExternalLinkIcon className="size-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>Open in new window</TooltipContent>
					</Tooltip>
				)}

				{/* Take/Release control */}
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant={isControlling ? "default" : "outline"}
							size="sm"
							onClick={isControlling ? onReleaseControl : onTakeControl}
							className="h-7 gap-1.5 px-2 text-xs"
						>
							{isControlling ? (
								<>
									<HandIcon className="size-3.5" />
									Release
								</>
							) : (
								<>
									<MousePointer2Icon className="size-3.5" />
									Control
								</>
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						{isControlling
							? "Release control of desktop"
							: "Take control of desktop"}
					</TooltipContent>
				</Tooltip>
			</div>
		</div>
	);
};
