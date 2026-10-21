import { HardDriveIcon } from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const WorkspaceFileChipBody: React.FC<{
	name: string;
	size?: number;
	title?: string;
	children?: React.ReactNode;
}> = ({ name, size, title, children }) => (
	<span
		title={title}
		className="flex max-w-64 items-center gap-1.5 rounded-md border border-solid border-border bg-surface-tertiary px-2.5 py-1.5 text-left text-xs"
	>
		<HardDriveIcon
			aria-hidden="true"
			className="size-3.5 shrink-0 text-content-secondary"
		/>
		<span className="truncate font-medium text-content-primary">{name}</span>
		{size !== undefined && size > 0 && (
			<span className="shrink-0 text-content-secondary">
				{prettyBytes(size)}
			</span>
		)}
		{children}
	</span>
);

/**
 * Timeline chip for a file sent to the chat's workspace. The file is for the
 * agent's workspace only, so the chip offers no preview or download. A chip
 * with a workspace path is a button so the path tooltip is keyboard-reachable.
 */
export const WorkspaceFileChip: React.FC<{
	name: string;
	path?: string;
	size?: number;
}> = ({ name, path, size }) => {
	if (!path) {
		// The name can truncate and there is no path tooltip, so the title shows it.
		return (
			<WorkspaceFileChipBody name={name} size={size} title={name}>
				<span className="shrink-0 text-content-secondary">
					For the workspace
				</span>
			</WorkspaceFileChipBody>
		);
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					className="cursor-default rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<WorkspaceFileChipBody name={name} size={size} />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top">
				<p className="max-w-xs break-all font-mono text-xs">{path}</p>
			</TooltipContent>
		</Tooltip>
	);
};
