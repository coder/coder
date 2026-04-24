import { UserIcon } from "lucide-react";
import { type FC, useId } from "react";
import type { AssignableRoles } from "#/api/typesGenerated";
import { Checkbox } from "#/components/Checkbox/Checkbox";

const roleDescriptions: Record<string, string> = {
	owner:
		"Owner can manage all resources, including users, groups, templates, and workspaces.",
	"user-admin": "User admin can manage all users and groups.",
	"template-admin": "Template admin can manage all templates and workspaces.",
	auditor: "Auditor can access the audit logs.",
	"agents-access": "Grants access to Coder Agents chat.",
	member:
		"Everybody is a member. This is a shared and default role for all users.",
};

interface RoleSelectorProps {
	roles: AssignableRoles[];
	selectedRoles: string[];
	onChange: (roles: string[]) => void;
}

export const RoleSelector: FC<RoleSelectorProps> = ({
	roles,
	selectedRoles,
	onChange,
}) => {
	const baseId = useId();
	const selectableRoles = roles.filter(
		(r) => r.assignable && r.name !== "member",
	);

	const handleToggle = (roleName: string) => {
		if (selectedRoles.includes(roleName)) {
			onChange(selectedRoles.filter((r) => r !== roleName));
		} else {
			onChange([...selectedRoles, roleName]);
		}
	};

	return (
		<div className="flex flex-col gap-2">
			<span className="text-sm font-medium">Roles</span>
			<div className="border border-border border-solid rounded-md">
				<div className="overflow-y-auto max-h-64 p-3 flex flex-col gap-2">
					{selectableRoles.map((role) => {
						const checkboxId = `${baseId}-${role.name}`;
						return (
							<label
								key={role.name}
								htmlFor={checkboxId}
								className="flex items-start gap-2 cursor-pointer"
							>
								<Checkbox
									id={checkboxId}
									checked={selectedRoles.includes(role.name)}
									onCheckedChange={() => handleToggle(role.name)}
									className="mt-1 shrink-0"
								/>
								<div className="flex flex-col">
									<span className="text-sm font-medium">
										{role.display_name || role.name}
									</span>
									<span className="text-sm text-content-secondary">
										{roleDescriptions[role.name] ?? ""}
									</span>
								</div>
							</label>
						);
					})}
				</div>
			</div>
			<div className="border-t border-border py-2 flex items-start gap-2 text-content-disabled">
				<UserIcon className="size-4 mt-1 shrink-0" />
				<div className="flex flex-col">
					<span className="text-sm font-medium">Member</span>
					<span className="text-sm">{roleDescriptions.member}</span>
				</div>
			</div>
		</div>
	);
};
