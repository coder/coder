import type { FC } from "react";
import type { Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

type OrganizationPickerProps = {
	/**
	 * The full list of organizations to render in the dropdown. Pass
	 * `undefined` while the list is loading.
	 */
	organizations: Organization[] | undefined;
	/**
	 * The id of the currently-selected organization. If the value is not
	 * present in `organizations` the trigger renders the placeholder.
	 */
	value: string;
	onValueChange?: (id: string) => void;
	/**
	 * Render the trigger as a non-interactive dropdown. The selected value is
	 * still visible. Used on the update-provider page today because providers
	 * aren't org-scoped on the wire yet, so the user can see which org the
	 * page entered from but can't reassign it.
	 */
	disabled?: boolean;
	/** Optional override of the trigger's `aria-label`. */
	ariaLabel?: string;
	/** Optional id for the trigger so a separate `<Label>` can target it. */
	id?: string;
	/**
	 * Override the trigger's width. Defaults to `"w-56 min-w-0"` to match the
	 * page-header usage; pass `"w-full"` when rendering inside a form column.
	 */
	triggerClassName?: string;
};

/**
 * Reusable organization picker for the AI Settings flow. The picker is
 * presentational today: AI providers are not yet organization-scoped on the
 * wire, so the selected id only drives URL state for the parent pages, not
 * the providers query.
 */
export const OrganizationPicker: FC<OrganizationPickerProps> = ({
	organizations,
	value,
	onValueChange,
	disabled = false,
	ariaLabel = "Organization",
	id,
	triggerClassName = "w-56 min-w-0",
}) => {
	const hasOrganizations = Boolean(organizations?.length);
	const isDisabled = disabled || !hasOrganizations;
	return (
		<Select
			value={hasOrganizations && value ? value : undefined}
			onValueChange={onValueChange}
			disabled={isDisabled}
		>
			<SelectTrigger
				id={id}
				className={triggerClassName}
				aria-label={ariaLabel}
			>
				<SelectValue placeholder="Select organization" />
			</SelectTrigger>
			<SelectContent>
				{organizations?.map((organization) => (
					<SelectItem key={organization.id} value={organization.id}>
						<span className="flex items-center gap-2">
							<Avatar
								variant="icon"
								size="sm"
								src={organization.icon}
								fallback={organization.display_name || organization.name}
							/>
							<span className="truncate">
								{organization.display_name || organization.name}
							</span>
						</span>
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
};
