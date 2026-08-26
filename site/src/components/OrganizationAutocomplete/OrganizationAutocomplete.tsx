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
import { cn } from "#/utils/cn";

type OrganizationAutocompleteProps = {
	value: Organization | null;
	onChange: (organization: Organization | null) => void;
	options: readonly Organization[];
	// Collision set for disambiguation labels. Lets callers include
	// organizations that are visible but not selectable, such as a
	// selected organization missing from options. Defaults to options.
	labelOrganizations?: readonly Organization[];
	id?: string;
	ariaLabel?: string;
	required?: boolean;
	disabled?: boolean;
	/**
	 * Overrides the trigger button's width/layout classes when the default
	 * full-width treatment does not fit (e.g. a fixed-width switcher).
	 */
	triggerClassName?: string;
	optionsTabbable?: boolean;
};

export const getOrganizationLabel = (
	organization: Organization,
	organizations: readonly Organization[],
) => {
	const displayName = organization.display_name || organization.name;
	const hasCollidingDisplayName = organizations.some(
		(other) =>
			other.id !== organization.id &&
			(other.display_name || other.name) === displayName,
	);

	if (hasCollidingDisplayName && organization.name !== displayName) {
		return `${displayName} (${organization.name})`;
	}
	return displayName;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	value,
	onChange,
	options,
	labelOrganizations,
	id,
	ariaLabel,
	required,
	disabled,
	triggerClassName,
	optionsTabbable = false,
}) => {
	const [open, setOpen] = useState(false);
	const labelContext = labelOrganizations ?? options;

	// GetOrganizations has no ORDER BY, so the caller needs a stable order.
	const sortedOptions = options.toSorted((a, b) => {
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
					aria-label={ariaLabel}
					variant="outline"
					disabled={disabled}
					aria-expanded={open}
					aria-required={required}
					data-testid="organization-autocomplete"
					className={cn(
						"group w-full justify-start gap-2 font-normal",
						triggerClassName,
					)}
				>
					{value ? (
						<>
							<Avatar
								size="sm"
								src={value.icon}
								fallback={value.display_name}
							/>
							<span className="truncate">
								{getOrganizationLabel(value, labelContext)}
							</span>
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
									tabIndex={optionsTabbable ? 0 : undefined}
								>
									<Avatar
										size="sm"
										src={org.icon}
										fallback={org.display_name}
									/>
									<span className="truncate">
										{getOrganizationLabel(org, labelContext)}
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

type OrganizationValueProps = {
	organization: Organization;
	labelOrganizations?: readonly Organization[];
	id?: string;
	className?: string;
};

// Static read-only counterpart to OrganizationAutocomplete.
export const OrganizationValue: FC<OrganizationValueProps> = ({
	organization,
	labelOrganizations,
	id,
	className,
}) => {
	const label = getOrganizationLabel(
		organization,
		labelOrganizations ?? [organization],
	);
	return (
		<div
			id={id}
			role="group"
			aria-label={`Organization ${label}`}
			className={cn(
				"flex h-10 items-center gap-2 rounded-md border border-solid border-border px-3 py-2 text-sm text-content-primary",
				className,
			)}
		>
			<Avatar
				size="sm"
				src={organization.icon}
				fallback={organization.display_name}
			/>
			<span className="truncate">{label}</span>
		</div>
	);
};
