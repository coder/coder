import Checkbox from "@mui/material/Checkbox";
import type { SlimRole } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { CollapsibleSummary } from "components/CollapsibleSummary/CollapsibleSummary";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { EditSquare } from "components/Icons/EditSquare";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { UserIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";

const roleDescriptions: Record<string, string> = {
	owner:
		"Owner can manage all resources, including users, groups, templates, and workspaces.",
	"user-admin": "User admin can manage all users and groups.",
	"template-admin": "Template admin can manage all templates and workspaces.",
	auditor: "Auditor can access the audit logs.",
	member:
		"Everybody is a member. This is a shared and default role for all users.",
};

interface OptionProps {
	value: string;
	name: string;
	description: string;
	isChecked: boolean;
	onChange: (roleName: string) => void;
}

const Option: FC<OptionProps> = ({
	value,
	name,
	description,
	isChecked,
	onChange,
}) => {
	return (
		<label htmlFor={name} className="cursor-pointer">
			<div className="flex items-start gap-4">
				<Checkbox
					id={name}
					size="small"
					className="p-0 relative top-px"
					value={value}
					checked={isChecked}
					onChange={(e) => {
						onChange(e.currentTarget.value);
					}}
				/>
				<div className="flex flex-col">
					<strong className="text-sm">{name}</strong>
					<span className="text-xs text-content-secondary">{description}</span>
				</div>
			</div>
		</label>
	);
};

interface EditRolesButtonProps {
	isLoading: boolean;
	roles: readonly SlimRole[];
	selectedRoleNames: Set<string>;
	onChange: (roles: SlimRole["name"][]) => void;
	oidcRoleSync: boolean;
	userLoginType?: string;
}

export const EditRolesButton: FC<EditRolesButtonProps> = (props) => {
	const { userLoginType, oidcRoleSync } = props;
	const canSetRoles =
		userLoginType !== "oidc" || (userLoginType === "oidc" && !oidcRoleSync);

	if (!canSetRoles) {
		return (
			<HelpTooltip>
				<HelpTooltipIconTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>Externally controlled</HelpTooltipTitle>
					<HelpTooltipText>
						Roles for this user are controlled by the OIDC identity provider.
					</HelpTooltipText>
				</HelpTooltipContent>
			</HelpTooltip>
		);
	}

	return <EnabledEditRolesButton {...props} />;
};

const EnabledEditRolesButton: FC<EditRolesButtonProps> = ({
	roles,
	selectedRoleNames,
	onChange,
	isLoading,
}) => {
	const handleChange = (roleName: string) => {
		if (selectedRoleNames.has(roleName)) {
			const serialized = [...selectedRoleNames];
			onChange(serialized.filter((role) => role !== roleName));
			return;
		}

		onChange([...selectedRoleNames, roleName]);
	};
	const [isAdvancedOpen, setIsAdvancedOpen] = useState(false);

	const filteredRoles = roles.filter(
		(role) => role.name !== "organization-workspace-creation-ban",
	);
	const advancedRoles = roles.filter(
		(role) => role.name === "organization-workspace-creation-ban",
	);

	// make sure the advanced roles are always visible if the user has one of these roles
	useEffect(() => {
		if (selectedRoleNames.has("organization-workspace-creation-ban")) {
			setIsAdvancedOpen(true);
		}
	}, [selectedRoleNames]);

	return (
		<Popover>
			<Tooltip>
				<PopoverTrigger asChild>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							aria-label="Edit user roles"
							size="icon"
							className="text-content-secondary hover:text-content-primary"
						>
							<EditSquare />
						</Button>
					</TooltipTrigger>
				</PopoverTrigger>
				<TooltipContent side="bottom">Edit user roles</TooltipContent>
			</Tooltip>

			<PopoverContent
				align="start"
				className="w-96 bg-surface-secondary border-surface-quaternary"
			>
				<fieldset
					className="border-0 m-0 p-0 disabled:opacity-50"
					disabled={isLoading}
					title="Available roles"
				>
					<div className="flex flex-col gap-4 p-6 w-96">
						{filteredRoles.map((role) => (
							<Option
								key={role.name}
								onChange={handleChange}
								isChecked={selectedRoleNames.has(role.name)}
								value={role.name}
								name={role.display_name || role.name}
								description={roleDescriptions[role.name] ?? ""}
							/>
						))}
						{advancedRoles.length > 0 && (
							<CollapsibleSummary label="advanced" defaultOpen={isAdvancedOpen}>
								{advancedRoles.map((role) => (
									<Option
										key={role.name}
										onChange={handleChange}
										isChecked={selectedRoleNames.has(role.name)}
										value={role.name}
										name={role.display_name || role.name}
										description={roleDescriptions[role.name] ?? ""}
									/>
								))}
							</CollapsibleSummary>
						)}
					</div>
				</fieldset>
				<div className="p-6 border-t-1 border-solid border-border text-sm">
					<div className="flex gap-4">
						<UserIcon />
						<div className="flex flex-col">
							<strong>Member</strong>
							<span className="text-xs text-content-secondary">
								{roleDescriptions.member}
							</span>
						</div>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
