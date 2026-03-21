import { checkAuthorization } from "api/queries/authCheck";
import { organizations } from "api/queries/organizations";
import type { AuthorizationCheck, Organization } from "api/typesGenerated";
import { ChevronDownIcon } from "components/AnimatedIcons/ChevronDown";
import { Avatar } from "components/Avatar/Avatar";
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
import { Check } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";

type OrganizationAutocompleteProps = {
	onChange: (organization: Organization | null) => void;
	id?: string;
	required?: boolean;
	check?: AuthorizationCheck;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	onChange,
	id,
	required,
	check,
}) => {
	const [open, setOpen] = useState(false);
	const [selected, setSelected] = useState<Organization | null>(null);

	const organizationsQuery = useQuery(organizations());

	const checks =
		check &&
		organizationsQuery.data &&
		Object.fromEntries(
			organizationsQuery.data.map((org) => [
				org.id,
				{
					...check,
					object: { ...check.object, organization_id: org.id },
				},
			]),
		);

	const permissionsQuery = useQuery({
		...checkAuthorization({ checks: checks ?? {} }),
		enabled: Boolean(check && organizationsQuery.data),
	});

	// If an authorization check was provided, filter the organizations based on
	// the results of that check.
	let options = organizationsQuery.data ?? [];
	if (check) {
		options = permissionsQuery.data
			? options.filter((org) => permissionsQuery.data[org.id])
			: [];
	}

	// Unfortunate: this useEffect sets a default org value
	// if only one is available and is necessary as the autocomplete loads
	// its own data. Until we refactor, proceed cautiously!
	useEffect(() => {
		const org = options[0];
		if (options.length !== 1 || org === selected) {
			return;
		}

		setSelected(org);
		onChange(org);
	}, [options, selected, onChange]);

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
					{selected ? (
						<>
							<Avatar
								size="sm"
								src={selected.icon}
								fallback={selected.display_name}
							/>
							<span className="truncate">{selected.display_name}</span>
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
										setSelected(org);
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
									{selected?.id === org.id && (
										<Check className="ml-auto size-icon-sm shrink-0" />
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
