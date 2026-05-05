import { UserIcon } from "lucide-react";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type { AssignableRoles } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { roleDescriptions } from "./index";

type RoleSelectorProps = {
	hideLabel?: boolean;
	loading?: boolean;
	error?: unknown;
	availableRoles?: AssignableRoles[];
	selectedRoles: Set<string>;
	onChange: (roles: Set<string>) => void;
};

export const RoleSelector: FC<RoleSelectorProps> = ({
	hideLabel,
	loading,
	error,
	availableRoles = [],
	selectedRoles,
	onChange,
}) => {
	const baseId = useId();
	const selectableRoles = availableRoles.filter((r) => r.name !== "member");

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

	if (selectableRoles.length === 0) {
		return null;
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
		<RoleSelectorLayout hideLabel={hideLabel}>
			{selectableRoles.length > 0 && (
				<div className="border border-border border-solid rounded-md overflow-y-auto max-h-72 p-3 flex flex-col gap-2">
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
									checked={selectedRoles.has(role.name)}
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
			)}

			<MemberRole />
		</RoleSelectorLayout>
	);
};

type RoleSelectorLayoutProps = {
	hideLabel?: boolean;
	children: React.ReactNode;
};

const RoleSelectorLayout: React.FC<RoleSelectorLayoutProps> = ({
	hideLabel,
	children,
}) => {
	return (
		<div className="flex flex-col gap-2">
			{!hideLabel && <span className="text-sm font-medium">Roles</span>}
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
