import { cn } from "cn";
import { FolderIcon, type LucideIcon, TriangleAlertIcon } from "lucide-react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ContextResourceTreeProps = { children: React.ReactNode };

/**
 * Read-only resource hierarchy. Description tooltips require an ancestor TooltipProvider.
 */
export const ContextResourceTree: React.FC<ContextResourceTreeProps> = ({
	children,
}) => <div className="flex min-w-0 w-full flex-col gap-2">{children}</div>;

type ContextResourceRowProps = {
	className?: string;
	icon?: LucideIcon;
	name: string;
	title?: string;
	description?: string;
	appearance?: "primary" | "secondary" | "path";
};

const ContextResourceRow: React.FC<ContextResourceRowProps> = ({
	icon: Icon,
	name,
	title,
	description,
	className,
	appearance = "primary",
}) => {
	const row = (
		<div
			className={cn(
				"flex min-w-0 items-center gap-1 py-px font-sans text-xs font-normal leading-4.5",
				appearance === "primary"
					? "text-content-primary"
					: "text-surface-invert-secondary",
				appearance === "path" &&
					"h-4.5 w-fit max-w-full justify-center rounded bg-surface-secondary px-1.5 py-1",
				className,
			)}
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
	return description ? (
		<Tooltip delayDuration={300}>
			<TooltipTrigger asChild>{row}</TooltipTrigger>
			<TooltipContent side="right" sideOffset={4} className="max-w-48 text-xs">
				{description}
			</TooltipContent>
		</Tooltip>
	) : (
		row
	);
};

/** Renders a resource row with optional description and shared typography. */
export const ContextResourceTreeItem: React.FC<ContextResourceRowProps> = (
	props,
) => (
	<div className="flex min-w-0 flex-col gap-1">
		<ContextResourceRow {...props} />
	</div>
);

type ContextResourceToolProps = Pick<
	ContextResourceRowProps,
	"name" | "description"
>;

/** Connects each tool to its server, ending the branch at the final tool. */
export const ContextResourceTool: React.FC<ContextResourceToolProps> = (
	props,
) => (
	<div
		className={cn(
			"relative min-w-0 pl-3",
			"before:absolute before:-top-1 before:bottom-1/2 before:left-0.75 before:w-1.5 before:rounded-bl-md before:border-0 before:border-b before:border-l before:border-solid before:border-border",
			"after:absolute after:-top-1 after:-bottom-1 after:left-0.75 after:border-0 after:border-l after:border-solid after:border-border last:after:hidden",
		)}
	>
		<ContextResourceRow {...props} appearance="secondary" className="py-0" />
	</div>
);

type ContextResourceGroupProps = {
	header: React.ReactNode;
	children: React.ReactNode;
	kind: "directory" | "server";
	className?: string;
};

const ContextResourceGroup: React.FC<ContextResourceGroupProps> = ({
	header,
	children,
	kind,
	className,
}) => (
	<div
		className={cn(
			"flex min-w-0 flex-col",
			kind === "directory" ? "gap-2" : "gap-1 ml-6.5",
			className,
		)}
	>
		{header}
		{children && (
			<div
				className={cn(
					"flex flex-col gap-1",
					kind === "directory" ? "ml-6.5" : "ml-0",
				)}
			>
				{children}
			</div>
		)}
	</div>
);

type ContextResourceTreeDirectoryProps = {
	className?: string;
	dir: string;
	children: React.ReactNode;
};

/** Omits the directory level for resources without a parent path. */
export const ContextResourceTreeDirectory: React.FC<
	ContextResourceTreeDirectoryProps
> = ({ dir, children, className }) =>
	dir ? (
		<ContextResourceGroup
			kind="directory"
			className={className}
			header={
				<ContextResourceRow
					icon={FolderIcon}
					name={dir}
					title={dir}
					appearance="path"
				/>
			}
		>
			{children}
		</ContextResourceGroup>
	) : (
		children
	);

type ContextResourceServerProps = {
	name: string;
	title: string;
	connected: boolean;
	children: React.ReactNode;
};

/** Keeps disconnected servers visible without exposing their tools. */
export const ContextResourceServer: React.FC<ContextResourceServerProps> = ({
	name,
	title,
	connected,
	children,
}) => (
	<ContextResourceGroup
		kind="server"
		header={
			connected ? (
				<ContextResourceRow name={name} title={title} />
			) : (
				<div className="flex min-w-0 items-center gap-1 text-content-disabled">
					<ContextResourceRow
						name={name}
						title={title}
						className="text-inherit"
					/>
					<TriangleAlertIcon aria-hidden="true" className="size-3 shrink-0" />
					<span className="sr-only">Not connected</span>
				</div>
			)
		}
	>
		{connected && children}
	</ContextResourceGroup>
);
