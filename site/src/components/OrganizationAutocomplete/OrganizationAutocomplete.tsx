import { CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
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

type OrganizationAutocompleteProps = {
	value: Organization | null;
	onChange: (organization: Organization | null) => void;
	options: readonly Organization[];
	id?: string;
	required?: boolean;
	/**
	 * Overrides the trigger button's width/layout classes when the default
	 * full-width treatment does not fit (e.g. a fixed-width switcher).
	 */
	triggerClassName?: string;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	value,
	onChange,
	options,
	id,
	required,
	triggerClassName,
}) => {
	const [open, setOpen] = useState(false);

	// Deterministic order: the active organization first, then
	// case-insensitive alphabetical by display name. GetOrganizations has
	// no ORDER BY, so source order is undefined.
	const sortedOptions = [...options].sort((a, b) => {
		if (a.id === value?.id) return -1;
		if (b.id === value?.id) return 1;
		return a.display_name
			.toLowerCase()
			.localeCompare(b.display_name.toLowerCase());
	});

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					id={id}
					variant="outline"
					aria-expanded={open}
					aria-required={required}
					data-testid="organization-autocomplete"
					className={
						triggerClassName ?? "w-full justify-start gap-2 font-normal"
					}
				>
					{value ? (
						<>
							<Avatar
								size="sm"
								src={value.icon}
								fallback={value.display_name}
							/>
							<span className="truncate">{value.display_name}</span>
						</>
					) : (
						<span className="text-content-secondary">
							Select an organization{required ? "…" : " (optional)"}
						</span>
					)}
					<ChevronDownIcon className="ml-auto !size-icon-sm shrink-0 text-content-secondary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				className="w-[var(--radix-popover-trigger-width)] p-0"
			>
				<Command loop>
					<CommandInput placeholder="Find organization…" />
					<CommandList>
						<CommandEmpty>No organizations found.</CommandEmpty>
						<CommandGroup>
							{sortedOptions.map((org) => (
								<CommandItem
									key={org.id}
									value={`${org.display_name} ${org.name}`}
									onSelect={() => {
										onChange(org);
										setOpen(false);
									}}
									// There is currently an issue with the cmdk component for keyboard navigation
									// https://github.com/pacocoursey/cmdk/issues/322
									tabIndex={0}
								>
									<Avatar
										size="sm"
										src={org.icon}
										fallback={org.display_name}
									/>
									<span className="truncate">
										{org.display_name || org.name}
									</span>
									{value?.id === org.id && (
										<CheckIcon className="ml-auto size-icon-sm shrink-0" />
									)}
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
