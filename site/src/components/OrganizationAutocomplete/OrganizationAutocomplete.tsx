import { useCommandState } from "cmdk";
import { cn } from "cn";
import { CheckIcon } from "lucide-react";
import { useId, useLayoutEffect, useRef, useState } from "react";
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
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type OrganizationAutocompleteProps = Pick<
	React.ComponentProps<"button">,
	"aria-invalid" | "aria-describedby"
> & {
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

export const OrganizationAutocomplete: React.FC<
	OrganizationAutocompleteProps
> = ({
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
	...ariaProps
}) => {
	const [open, setOpen] = useState(false);
	const contentId = useId();
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
					{...ariaProps}
					id={id}
					role="combobox"
					aria-label={ariaLabel}
					variant="outline"
					disabled={disabled}
					aria-expanded={open}
					aria-controls={contentId}
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
					<ChevronDownIcon className="ml-auto size-icon-sm! shrink-0 text-content-secondary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				id={contentId}
				align="start"
				className="w-(--radix-popover-trigger-width) p-0"
			>
				<Command loop label="Find organization">
					<CommandInput placeholder="Find organization…" />
					<CommandList>
						<CommandEmpty>No organizations found.</CommandEmpty>
						<CommandGroup>
							{sortedOptions.map((org) => (
								<OrganizationOption
									key={org.id}
									organization={org}
									label={getOrganizationLabel(org, labelContext)}
									isSelected={value?.id === org.id}
									onSelect={() => {
										onChange(org);
										setOpen(false);
									}}
									tabbable={optionsTabbable}
								/>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

type OrganizationOptionProps = {
	organization: Organization;
	label: string;
	isSelected: boolean;
	onSelect: () => void;
	tabbable: boolean;
};

const OrganizationOption: React.FC<OrganizationOptionProps> = ({
	organization,
	label,
	isSelected,
	onSelect,
	tabbable,
}) => {
	const value = `${organization.display_name} ${organization.name}`;
	const isHighlighted = useCommandState((state) => state.value === value);
	const labelRef = useRef<HTMLSpanElement>(null);
	const [isTooltipOpen, setIsTooltipOpen] = useState(false);
	const isLabelTruncated = () => {
		const span = labelRef.current;
		return span !== null && span.scrollWidth > span.clientWidth;
	};

	// The search input keeps focus unless options are tabbable, so the
	// tooltip also follows the cmdk highlight from the arrow keys. The
	// observer catches the popover settling to its final width.
	useLayoutEffect(() => {
		const span = labelRef.current;
		if (!isHighlighted || span === null) {
			setIsTooltipOpen(false);
			return;
		}
		const updateTooltip = () =>
			setIsTooltipOpen(span.scrollWidth > span.clientWidth);
		updateTooltip();
		const observer = new ResizeObserver(updateTooltip);
		observer.observe(span);
		return () => observer.disconnect();
	}, [isHighlighted]);

	return (
		<Tooltip
			open={isTooltipOpen}
			// Hover and focus requests only open a truncated label. Escape reaches
			// the tooltip before the popover, so the next Escape closes the popover.
			onOpenChange={(open) => setIsTooltipOpen(open && isLabelTruncated())}
		>
			<TooltipTrigger asChild>
				<CommandItem
					value={value}
					aria-selected={isSelected}
					onSelect={onSelect}
					tabIndex={tabbable ? 0 : undefined}
				>
					<Avatar
						size="sm"
						src={organization.icon}
						fallback={organization.display_name}
					/>
					<span ref={labelRef} className="truncate">
						{label}
					</span>
					{isSelected && (
						<CheckIcon className="ml-auto size-icon-sm shrink-0 text-content-primary" />
					)}
				</CommandItem>
			</TooltipTrigger>
			<TooltipContent className="max-w-65">{label}</TooltipContent>
		</Tooltip>
	);
};

type OrganizationValueProps = {
	organization: Organization;
	labelOrganizations?: readonly Organization[];
	id?: string;
	className?: string;
};

const OrganizationValue: React.FC<OrganizationValueProps> = ({
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
				"flex h-10 items-center gap-2 rounded-md border px-3 py-2 text-sm text-content-primary",
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

type OrganizationFieldProps = {
	id: string;
	organization: Organization;
	organizations: readonly Organization[];
	labelOrganizations?: readonly Organization[];
	onChange?: (organization: Organization) => void;
	className?: string;
	disabled?: boolean;
	label?: string;
	showLabel?: boolean;
	showSingleOrganization?: boolean;
	readOnly?: boolean;
	triggerClassName?: string;
	optionsTabbable?: boolean;
	required?: boolean;
};

export const OrganizationField: React.FC<OrganizationFieldProps> = ({
	id,
	organization,
	organizations,
	labelOrganizations,
	onChange,
	className,
	disabled,
	label = "Organization",
	showLabel = true,
	showSingleOrganization = false,
	readOnly = false,
	triggerClassName,
	optionsTabbable,
	required = true,
}) => {
	const hasSingleSelectedOrganization =
		organizations.length <= 1 &&
		organizations.some((option) => option.id === organization.id);
	if (hasSingleSelectedOrganization && !showSingleOrganization && !readOnly) {
		return null;
	}

	const resolvedLabelOrganizations =
		labelOrganizations ??
		(organizations.some((option) => option.id === organization.id)
			? organizations
			: [...organizations, organization]);
	const organizationLabel = getOrganizationLabel(
		organization,
		resolvedLabelOrganizations,
	);
	const isReadOnly = readOnly || !onChange || hasSingleSelectedOrganization;

	return (
		<div className={cn("flex w-72 flex-col gap-1.5", className)}>
			{showLabel && (
				<Label
					htmlFor={id}
					className="flex items-center gap-1 leading-6 text-content-primary"
				>
					{label}
				</Label>
			)}
			{isReadOnly ? (
				<OrganizationValue
					id={id}
					organization={organization}
					labelOrganizations={resolvedLabelOrganizations}
				/>
			) : (
				<OrganizationAutocomplete
					id={id}
					ariaLabel={`${label} ${organizationLabel}`}
					value={organization}
					onChange={(org) => {
						if (org) {
							onChange?.(org);
						}
					}}
					options={organizations}
					labelOrganizations={resolvedLabelOrganizations}
					required={required}
					disabled={disabled}
					triggerClassName={triggerClassName}
					optionsTabbable={optionsTabbable}
				/>
			)}
		</div>
	);
};
