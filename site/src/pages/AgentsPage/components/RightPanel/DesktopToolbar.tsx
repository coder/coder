import {
	AppWindowIcon,
	ExternalLinkIcon,
	HandIcon,
	MaximizeIcon,
	MinimizeIcon,
	MousePointer2Icon,
} from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAppLink } from "#/modules/apps/useAppLink";

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
 * A single app entry inside the Apps dropdown menu.
 */
const AppMenuItem: FC<{
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	workspace: TypesGen.Workspace;
}> = ({ app, agent, workspace }) => {
	const link = useAppLink(app, { agent, workspace });

	return (
		<DropdownMenuItem asChild>
			<a
				href={link.href}
				onClick={link.onClick}
				target="_blank"
				rel="noreferrer"
				className="flex items-center gap-2"
			>
				{app.icon ? (
					<ExternalImage src={app.icon} alt="" className="size-4 shrink-0" />
				) : (
					<AppWindowIcon className="size-4 shrink-0 text-content-secondary" />
				)}
				{link.label}
			</a>
		</DropdownMenuItem>
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
	const apps = agent?.apps.filter((app) => !app.hidden) ?? [];

	return (
		<div
			className="absolute top-0 right-0 left-0 z-20 flex h-8 items-center justify-between border-0 border-b border-solid border-border-default bg-surface-primary px-1.5"
			role="toolbar"
			aria-label="Desktop controls"
		>
			{/* Left: Apps dropdown */}
			<div className="flex items-center gap-1">
				{agent && workspace && apps.length > 0 && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="subtle"
								size="sm"
								className="h-6 gap-1 px-2 text-xs text-content-secondary hover:text-content-primary"
							>
								<AppWindowIcon className="size-3.5" />
								Apps
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="start">
							{apps.map((app) => (
								<AppMenuItem
									key={app.slug}
									app={app}
									agent={agent}
									workspace={workspace}
								/>
							))}
						</DropdownMenuContent>
					</DropdownMenu>
				)}
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
							className="h-6 w-6 text-content-secondary hover:text-content-primary"
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
								className="h-6 w-6 text-content-secondary hover:text-content-primary"
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
							className="h-6 gap-1.5 px-2 text-xs"
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
