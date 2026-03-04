import { workspaceById, workspaces } from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { useDebouncedValue } from "hooks/debounce";
import { CheckIcon, ChevronsUpDownIcon, MonitorIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { cn } from "utils/cn";

const autoCreateWorkspaceValue = "__auto_create_workspace__";

type WorkspaceComboboxProps = {
	value: string | null;
	onValueChange: (value: string | null) => void;
	disabled?: boolean;
};

export const WorkspaceCombobox: FC<WorkspaceComboboxProps> = ({
	value,
	onValueChange,
	disabled = false,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search, 250);

	const searchQuery = debouncedSearch
		? `owner:me ${debouncedSearch}`
		: "owner:me";

	const { data: workspacesData, isFetched } = useQuery({
		...workspaces({ q: searchQuery }),
		placeholderData: keepPreviousData,
	});

	const workspaceList = workspacesData?.workspaces ?? [];

	// If a workspace is selected but not in current search results, fetch it
	// separately so the trigger label is always correct and it appears in the list.
	const selectedNotInList =
		value !== null && !workspaceList.some((ws) => ws.id === value);
	const { data: selectedWorkspaceData } = useQuery({
		...workspaceById(value ?? ""),
		enabled: selectedNotInList && value !== null,
	});

	// Build the final options list, ensuring the selected workspace is included.
	const options: readonly Workspace[] =
		selectedNotInList && selectedWorkspaceData
			? [selectedWorkspaceData, ...workspaceList]
			: workspaceList;

	const selectedWorkspace = value
		? options.find((ws) => ws.id === value)
		: undefined;
	const triggerLabel = selectedWorkspace
		? `${selectedWorkspace.owner_name}/${selectedWorkspace.name}`
		: "Workspace";

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					disabled={disabled || !isFetched}
					role="combobox"
					aria-expanded={open}
					className="h-8 w-auto gap-1.5 border-none bg-transparent px-1 text-xs shadow-none transition-colors hover:bg-transparent hover:text-content-primary [&>svg]:transition-colors [&>svg]:hover:text-content-primary focus:ring-0 focus-visible:ring-0"
				>
					<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary group-hover:text-content-primary" />
					<span className="max-w-[200px] truncate">{triggerLabel}</span>
					<ChevronsUpDownIcon className="h-3 w-3 shrink-0 opacity-50" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				className="flex flex-col w-[280px] p-0"
				side="top"
				align="start"
			>
				<Command className="flex-1 min-h-0" shouldFilter={false}>
					<CommandInput
						placeholder="Search workspaces..."
						value={search}
						onValueChange={setSearch}
						aria-label="Search workspaces"
					/>
					<CommandList className="flex-1 min-h-0 max-h-64 overflow-y-auto">
						<CommandEmpty>No workspaces found.</CommandEmpty>
						<CommandGroup>
							<CommandItem
								value={autoCreateWorkspaceValue}
								onSelect={() => {
									onValueChange(null);
									setOpen(false);
								}}
							>
								Auto-create Workspace
								<CheckIcon
									className={cn(
										"ml-auto h-4 w-4",
										value === null ? "opacity-100" : "opacity-0",
									)}
								/>
							</CommandItem>
							{options.map((workspace) => (
								<CommandItem
									key={workspace.id}
									keywords={[workspace.name, workspace.owner_name]}
									value={workspace.id}
									onSelect={() => {
										onValueChange(workspace.id);
										setOpen(false);
									}}
								>
									{workspace.owner_name}/{workspace.name}
									<CheckIcon
										className={cn(
											"ml-auto h-4 w-4",
											value === workspace.id ? "opacity-100" : "opacity-0",
										)}
									/>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
