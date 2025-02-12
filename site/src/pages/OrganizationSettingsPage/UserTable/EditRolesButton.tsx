import UserIcon from "@mui/icons-material/PersonOutline";
import Checkbox from "@mui/material/Checkbox";
import Tooltip from "@mui/material/Tooltip";
import type { SlimRole } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { EditSquare } from "components/Icons/EditSquare";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import type { FC } from "react";

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
					<strong>{name}</strong>
					<span className="text-xs text-content-secondary">{description}</span>
				</div>
			</div>
		</label>
	);
};

export interface EditRolesButtonProps {
	isLoading: boolean;
	roles: readonly SlimRole[];
	selectedRoleNames: Set<string>;
	onChange: (roles: SlimRole["name"][]) => void;
	oidcRoleSync: boolean;
	userLoginType?: string;
}

export const EditRolesButton: FC<EditRolesButtonProps> = ({
	roles,
	selectedRoleNames,
	onChange,
	isLoading,
	userLoginType,
	oidcRoleSync,
}) => {
	const handleChange = (roleName: string) => {
		if (selectedRoleNames.has(roleName)) {
			const serialized = [...selectedRoleNames];
			onChange(serialized.filter((role) => role !== roleName));
			return;
		}

		onChange([...selectedRoleNames, roleName]);
	};

	const canSetRoles =
		userLoginType !== "oidc" || (userLoginType === "oidc" && !oidcRoleSync);

	if (!canSetRoles) {
		return (
			<HelpTooltip>
				<HelpTooltipTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>Externally controlled</HelpTooltipTitle>
					<HelpTooltipText>
						Roles for this user are controlled by the OIDC identity provider.
					</HelpTooltipText>
				</HelpTooltipContent>
			</HelpTooltip>
		);
	}

	return (
		<Popover>
			<PopoverTrigger>
				<Tooltip title="Edit user roles">
					<Button
						variant="subtle"
						aria-label="Edit user roles"
						size="icon"
						className="text-content-secondary hover:text-content-primary"
					>
						<EditSquare />
					</Button>
				</Tooltip>
			</PopoverTrigger>

			<PopoverContent className="w-80" disablePortal={false}>
				<fieldset
					className="border-0 m-0 p-0 disabled:opacity-50"
					disabled={isLoading}
					title="Available roles"
				>
					<div className="flex flex-col gap-4 p-6">
						{roles.map((role) => (
							<Option
								key={role.name}
								onChange={handleChange}
								isChecked={selectedRoleNames.has(role.name)}
								value={role.name}
								name={role.display_name || role.name}
								description={roleDescriptions[role.name] ?? ""}
							/>
						))}
					</div>
				</fieldset>
				<div className="p-6 border-t-1 border-solid border-border text-sm">
					<div className="flex gap-4">
						<UserIcon className="size-icon-sm" />
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
