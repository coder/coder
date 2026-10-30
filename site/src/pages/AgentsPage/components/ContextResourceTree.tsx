import { cn } from "cn";
import { FolderIcon, type LucideIcon } from "lucide-react";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ContextResourceTreeProps = { children: React.ReactNode };

/** Shares spacing and tooltip timing across context resource branches. */
export const ContextResourceTree: React.FC<ContextResourceTreeProps> = ({
	children,
}) => (
	<TooltipProvider delayDuration={300}>
		<div className="flex flex-col gap-1.5">{children}</div>
	</TooltipProvider>
);

type ContextResourceTreeItemProps = {
	className?: string;
	icon?: LucideIcon;
	name: string;
	title?: string;
	description?: string;
	children?: React.ReactNode;
};

/** Keeps nested resources aligned and optionally describes a row on hover. */
export const ContextResourceTreeItem: React.FC<
	ContextResourceTreeItemProps
> = ({ icon: Icon, name, title, description, children, className }) => {
	const row = (
		<div
			className={cn("flex min-w-0 items-center gap-1 py-px", className)}
			title={title}
		>
			{Icon && (
				<span className="flex size-4 shrink-0 items-center justify-center">
					<Icon className="size-3" />
				</span>
			)}
			<span className="truncate">{name}</span>
		</div>
	);
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			{description ? (
				<Tooltip>
					<TooltipTrigger asChild>{row}</TooltipTrigger>
					<TooltipContent
						side="right"
						sideOffset={4}
						className="max-w-48 text-xs"
					>
						{description}
					</TooltipContent>
				</Tooltip>
			) : (
				row
			)}
			{children && <div className="ml-5 flex flex-col gap-1.5">{children}</div>}
		</div>
	);
};

type ContextResourceTreeDirectoryProps = {
	dir: string;
	children: React.ReactNode;
};

/** Omits the directory level for resources without a parent path. */
export const ContextResourceTreeDirectory: React.FC<
	ContextResourceTreeDirectoryProps
> = ({ dir, children }) =>
	dir ? (
		<ContextResourceTreeItem icon={FolderIcon} name={dir} title={dir}>
			{children}
		</ContextResourceTreeItem>
	) : (
		children
	);
