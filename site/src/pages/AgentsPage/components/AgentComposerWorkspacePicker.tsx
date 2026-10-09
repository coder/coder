import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronRightIcon,
	MonitorIcon,
} from "lucide-react";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useAgentComposerOptions } from "./AgentComposerOptionsContext";

/** Opens the desktop picker or switches the options menu to its mobile view. */
export const AgentComposerWorkspacePicker = ({
	isMobile,
	open,
	onOpenChange,
	disabled,
	onOpenMobile,
	onSelect,
}: {
	isMobile: boolean;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	disabled: boolean;
	onOpenMobile: () => void;
	onSelect: (id: string | null) => void;
}) => {
	const trigger = (
		<button
			type="button"
			disabled={disabled}
			onClick={isMobile ? onOpenMobile : undefined}
			className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
		>
			<MonitorIcon className="size-3.5 shrink-0" />
			<span>Attach workspace</span>
			<ChevronRightIcon
				className={cn(
					"ml-auto size-icon-sm",
					!isMobile && "transition-transform",
					!isMobile && open && "rotate-180",
				)}
			/>
		</button>
	);

	if (isMobile) {
		return trigger;
	}

	return (
		<Popover open={open} onOpenChange={onOpenChange}>
			<PopoverTrigger asChild>{trigger}</PopoverTrigger>
			<PopoverContent
				side="right"
				align="start"
				sideOffset={8}
				className="w-64 p-0"
			>
				<AgentComposerWorkspaceView
					onSelect={(id) => {
						onOpenChange(false);
						onSelect(id);
					}}
				/>
			</PopoverContent>
		</Popover>
	);
};

/** Searchable workspace choices, with a back action in the mobile menu. */
// Cross-organization workspaces remain selectable only when already selected,
// so stale bindings can still be cleared.
export const AgentComposerWorkspaceView = ({
	onSelect,
	onBack,
}: {
	onSelect: (id: string | null) => void;
	onBack?: () => void;
}) => {
	const { workspaceSelection, organizationId } = useAgentComposerOptions();

	if (!workspaceSelection) {
		return null;
	}

	const { options, selectedId } = workspaceSelection;

	return (
		<>
			{onBack && (
				<>
					<button
						type="button"
						onClick={onBack}
						className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
					>
						<ArrowLeftIcon className="size-3.5 shrink-0" />
						<span>Back</span>
					</button>
					<Separator className="my-1" />
				</>
			)}
			<Command loop>
				<CommandInput placeholder="Search workspaces..." className="text-xs" />
				<CommandList>
					<CommandEmpty className="text-xs">No workspaces found</CommandEmpty>
					<CommandGroup>
						{options.map((workspace) => {
							const isCrossOrg =
								!!organizationId &&
								workspace.organization_id !== organizationId;
							const isSelected = selectedId === workspace.id;
							const isUnavailable = isCrossOrg && !isSelected;

							const item = (
								<CommandItem
									className={cn(
										"text-xs font-normal",
										isUnavailable &&
											"cursor-not-allowed opacity-50 data-[disabled=true]:pointer-events-auto",
									)}
									key={workspace.id}
									value={workspace.name}
									disabled={isUnavailable}
									onSelect={() => {
										if (!isUnavailable) {
											onSelect(isSelected ? null : workspace.id);
										}
									}}
								>
									{workspace.name}
									{isSelected && (
										<CheckIcon className="ml-auto size-icon-sm shrink-0" />
									)}
								</CommandItem>
							);

							return isUnavailable ? (
								<Tooltip key={workspace.id}>
									<TooltipTrigger asChild>
										<div>{item}</div>
									</TooltipTrigger>
									<TooltipContent side="top">
										Chat and workspace must be in the same organization
									</TooltipContent>
								</Tooltip>
							) : (
								item
							);
						})}
					</CommandGroup>
				</CommandList>
			</Command>
		</>
	);
};
