import { HardDriveIcon } from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

/**
 * Timeline chip for a workspace file, whose bytes live in the workspace or in
 * Coder until it copies them there, so there is nothing to preview or
 * download. The trigger is a button so the tooltip is keyboard-reachable.
 */
export const WorkspaceFileChip: React.FC<{
	name: string;
	path?: string;
	size?: number;
}> = ({ name, path, size }) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<button
				type="button"
				className="flex max-w-64 cursor-default items-center gap-1.5 rounded-md border border-solid border-border bg-surface-tertiary px-2.5 py-1.5 text-left text-xs focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			>
				<HardDriveIcon
					aria-hidden="true"
					className="size-3.5 shrink-0 text-content-secondary"
				/>
				<span className="truncate font-medium text-content-primary">
					{name}
				</span>
				{size !== undefined && size > 0 && (
					<span className="shrink-0 text-content-secondary">
						{prettyBytes(size)}
					</span>
				)}
				{!path && (
					<span className="shrink-0 text-content-secondary">
						For the workspace
					</span>
				)}
			</button>
		</TooltipTrigger>
		<TooltipContent side="top">
			<p className="max-w-xs break-all font-mono text-xs">{path ?? name}</p>
		</TooltipContent>
	</Tooltip>
);
