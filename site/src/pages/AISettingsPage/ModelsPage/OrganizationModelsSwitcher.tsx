import { CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Organization } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
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

interface OrganizationModelsSwitcherProps {
	activeOrganization: Organization;
	organizations: readonly Organization[];
}

/**
 * Combobox that switches the /ai/settings/organizations/:organization/models
 * pages between the organizations the caller can manage.
 */
export const OrganizationModelsSwitcher: FC<
	OrganizationModelsSwitcherProps
> = ({ activeOrganization, organizations }) => {
	const [isPopoverOpen, setIsPopoverOpen] = useState(false);
	const navigate = useNavigate();
	const location = useLocation();
	// The switcher serves every subpage under the organization route; keep
	// the current subpage when swapping the organization segment.
	const subpagePath = location.pathname.replace(
		/^\/ai\/settings\/organizations\/[^/]+/,
		"",
	);

	const sortedOrganizations = [...organizations].sort((a, b) => {
		if (a.id === activeOrganization.id) return -1;
		if (b.id === activeOrganization.id) return 1;
		return a.display_name
			.toLowerCase()
			.localeCompare(b.display_name.toLowerCase());
	});

	return (
		<Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					aria-expanded={isPopoverOpen}
					className="w-60 gap-2 justify-start"
				>
					<Avatar
						size="sm"
						src={activeOrganization.icon}
						fallback={activeOrganization.display_name}
					/>
					<span className="truncate">
						{activeOrganization.display_name || activeOrganization.name}
					</span>
					<ChevronDownIcon className="ml-auto !size-icon-sm" />
				</Button>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-60">
				<Command loop>
					<CommandInput placeholder="Find organization" />
					<CommandList>
						<CommandEmpty>No organization found.</CommandEmpty>
						<CommandGroup className="pb-2">
							<div className="flex flex-col max-h-[260px] overflow-y-auto">
								{sortedOrganizations.map((organization) => (
									<CommandItem
										key={organization.id}
										value={`${organization.display_name} ${organization.name}`}
										onSelect={() => {
											setIsPopoverOpen(false);
											navigate(
												`/ai/settings/organizations/${organization.name}${subpagePath}`,
											);
										}}
										// There is currently an issue with the cmdk component for keyboard navigation
										// https://github.com/pacocoursey/cmdk/issues/322
										tabIndex={0}
									>
										<Avatar
											size="sm"
											src={organization.icon}
											fallback={organization.display_name}
										/>
										<span className="truncate">
											{organization.display_name || organization.name}
										</span>
										{activeOrganization.name === organization.name && (
											<CheckIcon className="ml-auto" />
										)}
									</CommandItem>
								))}
							</div>
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
