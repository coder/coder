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
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	value,
	onChange,
	options,
	id,
	required,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					id={id}
					variant="outline"
					aria-expanded={open}
					aria-required={required}
					data-testid="organization-autocomplete"
					className="w-full justify-start gap-2 font-normal"
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
							{options.map((org) => (
								<CommandItem
									key={org.id}
									value={`${org.display_name} ${org.name}`}
									onSelect={() => {
										onChange(org);
										setOpen(false);
									}}
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
