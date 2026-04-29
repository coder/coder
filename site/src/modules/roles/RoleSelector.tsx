import { UserIcon } from "lucide-react";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type { AssignableRoles } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { roleDescriptions } from "./index";

interface RoleSelectorProps {
	roles: AssignableRoles[];
	selectedRoles: Set<string>;
	onChange: (roles: Set<string>) => void;
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
	const selectableRoles = roles.filter(
		(r) => r.assignable && r.name !== "member",
	);

	if (loading) {
		return (
			<RoleSelectorLayout>
				<RoleSelectorSkeleton />
				<MemberRole />
			</RoleSelectorLayout>
		);
	}

	if (error) {
		return (
			<RoleSelectorLayout>
				<Alert severity="error">
					<AlertTitle>
						{getErrorMessage(error, "Failed to load roles.")}
					</AlertTitle>
				</Alert>
			</RoleSelectorLayout>
		);
	}

	const handleToggle = (roleName: string) => {
		const newRoles = new Set(selectedRoles);
		if (newRoles.has(roleName)) {
			newRoles.delete(roleName);
		} else {
			newRoles.add(roleName);
		}
		onChange(newRoles);
	};

	return (
		<RoleSelectorLayout>
			{selectableRoles.length > 0 && (
				<div className="border border-border border-solid rounded-md">
					<div className="overflow-y-auto max-h-72 p-3 flex flex-col gap-2">
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
										checked={selectedRoles.has(role.name)}
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
			)}

			<MemberRole />
		</RoleSelectorLayout>
	);
};

const RoleSelectorLayout: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<div className="flex flex-col gap-2">
			<span className="text-sm font-medium">Roles</span>
			{children}
		</div>
	);
};

const MemberRole: React.FC = () => {
	return (
		<div className="border-t border-border py-2 flex items-start gap-2 text-content-disabled">
			<UserIcon className="size-4 mt-1 shrink-0" />
			<div className="flex flex-col">
				<span className="text-sm font-medium">Member</span>
				<span className="text-sm">{roleDescriptions.member}</span>
			</div>
		</div>
	);
};

const RoleSelectorSkeleton: React.FC = () => {
	return (
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
	);
};
