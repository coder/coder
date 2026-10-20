import { cn } from "cn";
import { HardDriveIcon } from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const chipClassName =
	"flex max-w-64 items-center gap-1.5 rounded-md border border-solid border-border bg-surface-tertiary px-2.5 py-1.5 text-left text-xs";

const ChipContent: React.FC<{ name: string; size: number; label?: string }> = ({
	name,
	size,
	label,
}) => (
	<>
		<HardDriveIcon
			aria-hidden="true"
			className="size-3.5 shrink-0 text-content-secondary"
		/>
		<span className="truncate font-medium text-content-primary">{name}</span>
		{size > 0 && (
			<span className="shrink-0 text-content-secondary">
				{prettyBytes(size)}
			</span>
		)}
		{label && <span className="shrink-0 text-content-secondary">{label}</span>}
	</>
);

/**
 * Timeline chip for a workspace file. A path gets a keyboard-reachable
 * tooltip; without one, Coder copies the file in, so the chip is static.
 */
export const WorkspaceFileChip: React.FC<{
	name: string;
	path?: string;
	size: number;
}> = ({ name, path, size }) => {
	if (!path) {
		return (
			<div className={chipClassName}>
				<ChipContent name={name} size={size} label="For the workspace" />
			</div>
		);
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					className={cn(
						chipClassName,
						"cursor-default focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
					)}
				>
					<ChipContent name={name} size={size} />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top">
				<p className="max-w-xs break-all font-mono text-xs">{path}</p>
			</TooltipContent>
		</Tooltip>
	);
};
