import { useRef, useState } from "react";
import type {
	MinimalOrganization,
	UserOrganizationAISpend,
} from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
} from "#/components/DropdownMenu/DropdownMenu";
import { SeverityBadge, severityIndicators } from "./severityIndicators";
import { getSpendSeverity, toViewedSpend } from "./viewedSpend";

type UserDropdownOrgSelectProps = {
	organizations: readonly UserOrganizationAISpend[];
	selected: MinimalOrganization;
	onSelectedOrgIdChange: (orgId: string) => void;
};

const orgName = (org: MinimalOrganization) => org.display_name || org.name;

type OrgAvatarProps = { org: MinimalOrganization };

const OrgAvatar: React.FC<OrgAvatarProps> = ({ org }) => (
	// Decorative: the organization name is rendered next to it.
	<Avatar aria-hidden size="sm" src={org.icon} fallback={orgName(org)} />
);

/**
 * Picks which organization's AI spend the user menu shows. The list expands
 * inline so the menu stays open while the user switches organizations.
 */
export const UserDropdownOrgSelect: React.FC<UserDropdownOrgSelectProps> = ({
	organizations,
	selected,
	onSelectedOrgIdChange,
}) => {
	const [open, setOpen] = useState(false);
	const triggerRef = useRef<HTMLDivElement>(null);

	return (
		<>
			<DropdownMenuItem
				ref={triggerRef}
				aria-expanded={open}
				aria-label={`Organization: ${orgName(selected)}`}
				onSelect={(event) => {
					event.preventDefault();
					setOpen(!open);
				}}
			>
				<OrgAvatar org={selected} />
				<span className="min-w-0 flex-1 truncate">{orgName(selected)}</span>
				<ChevronDownIcon
					aria-hidden
					open={open}
					className="text-content-secondary"
				/>
			</DropdownMenuItem>
			{open && (
				<DropdownMenuRadioGroup
					className="max-h-64 overflow-y-auto"
					value={selected.id}
					onValueChange={(orgId) => {
						onSelectedOrgIdChange(orgId);
						setOpen(false);
						// The picked item unmounts with the list, so keep keyboard
						// focus inside the menu.
						triggerRef.current?.focus();
					}}
				>
					{organizations.map((entry) => {
						const indicator =
							severityIndicators[getSpendSeverity(toViewedSpend(entry))];
						return (
							<DropdownMenuRadioItem
								key={entry.organization.id}
								value={entry.organization.id}
								className="gap-2 font-normal"
								onSelect={(event) => event.preventDefault()}
							>
								<OrgAvatar org={entry.organization} />
								<span className="min-w-0 truncate">
									{orgName(entry.organization)}
								</span>
								{indicator && (
									<SeverityBadge indicator={indicator}>
										<span className="sr-only">. {indicator.label}</span>
									</SeverityBadge>
								)}
							</DropdownMenuRadioItem>
						);
					})}
				</DropdownMenuRadioGroup>
			)}
		</>
	);
};
