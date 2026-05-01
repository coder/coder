import { UserIcon } from "lucide-react";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type { AssignableRoles } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";

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
	loading?: boolean;
	error?: unknown;
}

export const RoleSelector: FC<RoleSelectorProps> = ({
	roles,
	selectedRoles,
	onChange,
	loading,
	error,
}) => {
	const baseId = useId();
	const selectableRoles = roles.filter((r) => r.name !== "member");

	const handleToggle = (roleName: string) => {
		if (selectedRoles.includes(roleName)) {
			onChange(selectedRoles.filter((r) => r !== roleName));
		} else {
			onChange([...selectedRoles, roleName]);
		}
	};

	if (loading) {
		return (
			<div className="flex flex-col gap-2">
				<span className="text-sm font-medium">Roles</span>
				<div className="border border-border border-solid rounded-md">
					<div className="p-3 flex flex-col gap-2">
						{Array.from({ length: 4 }, (_, i) => (
							<div key={i} className="flex items-start gap-2">
								<Skeleton className="mt-1 shrink-0 size-4 rounded" />
								<div className="flex flex-col gap-1 flex-1">
									<Skeleton variant="text" className="w-24" />
									<Skeleton variant="text" className="w-48" />
								</div>
							</div>
						))}
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
	}

	if (error) {
		return (
			<div className="flex flex-col gap-2">
				<span className="text-sm font-medium">Roles</span>
				<Alert severity="error">
					<AlertTitle>
						{getErrorMessage(error, "Failed to load roles.")}
					</AlertTitle>
				</Alert>
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-2">
			<span className="text-sm font-medium">Roles</span>
			{selectableRoles.length > 0 && (
				<div className="border border-border border-solid rounded-md">
					<div className="overflow-y-auto max-h-72 p-3 flex flex-col gap-2">
						{selectableRoles.map((role) => {
							const checkboxId = `${baseId}-${role.name}`;
							return (
								<label
									key={role.name}
									htmlFor={checkboxId}
									className={cn(
										"flex items-start gap-2",
										role.assignable
											? "cursor-pointer"
											: "cursor-not-allowed opacity-50",
									)}
								>
									<Checkbox
										id={checkboxId}
										checked={selectedRoles.includes(role.name)}
										onCheckedChange={() => handleToggle(role.name)}
										disabled={!role.assignable}
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
			)}
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
