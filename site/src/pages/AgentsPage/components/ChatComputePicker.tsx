import { ChevronDownIcon, MonitorIcon } from "lucide-react";
import { useState } from "react";
import type { Workspace } from "#/api/typesGenerated";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { chatTemplateOptions } from "./ChatTemplatePicker";

/** Picks an existing workspace or a template for a custom harness. */
export const ChatComputePicker = ({
	workspaceOptions,
	isWorkspaceLoading,
	chatOrganizationId,
	onWorkspaceChange,
	onTemplateChange,
}: {
	workspaceOptions?: readonly Pick<
		Workspace,
		"id" | "name" | "owner_name" | "organization_id"
	>[];
	isWorkspaceLoading?: boolean;
	chatOrganizationId?: string;
	onWorkspaceChange?: (id: string) => void;
	onTemplateChange: (id: string) => void;
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full border-0 bg-white px-2 py-0.5 text-xs font-medium text-black transition-colors hover:bg-white/90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-content-link"
				>
					<MonitorIcon className="size-3" />
					Compute
					<ChevronDownIcon className="size-3" />
				</button>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				align="start"
				className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-72 p-0"
			>
				<Command label="Search compute" loop>
					<CommandInput
						placeholder="Search workspaces and templates..."
						aria-label="Search compute"
						className="text-xs"
					/>
					<CommandList label="Compute">
						<CommandEmpty className="text-xs">
							No workspaces or templates found
						</CommandEmpty>
						<CommandGroup heading="Workspaces">
							{isWorkspaceLoading ? (
								<p
									role="status"
									className="m-0 px-2 py-2 text-xs text-content-secondary"
								>
									Loading workspaces...
								</p>
							) : workspaceOptions?.length ? (
								workspaceOptions.map((workspace) => {
									const isCrossOrg = Boolean(
										chatOrganizationId &&
											workspace.organization_id !== chatOrganizationId,
									);
									return (
										<CommandItem
											key={workspace.id}
											value={`workspace ${workspace.name}`}
											disabled={!onWorkspaceChange || isCrossOrg}
											onSelect={() => {
												onWorkspaceChange?.(workspace.id);
												setOpen(false);
											}}
											className="text-xs font-normal"
										>
											{workspace.name}
										</CommandItem>
									);
								})
							) : (
								<p className="m-0 px-2 py-2 text-xs text-content-secondary">
									No existing workspaces. Select a template below.
								</p>
							)}
						</CommandGroup>
						<CommandGroup heading="Templates">
							{chatTemplateOptions.map((template) => (
								<CommandItem
									key={template.id}
									value={`template ${template.name}`}
									onSelect={() => {
										onTemplateChange(template.id);
										setOpen(false);
									}}
									className="text-xs font-normal"
								>
									<ExternalImage
										src={template.icon}
										alt=""
										className="size-3.5 shrink-0"
									/>
									<span>{template.name}</span>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
