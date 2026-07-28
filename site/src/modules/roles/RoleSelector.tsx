import { UserIcon } from "lucide-react";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type { AssignableRoles } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { CollapsibleSummary } from "#/components/CollapsibleSummary/CollapsibleSummary";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { roleDescriptions } from "./index";

const advancedRoleNames = ["organization-workspace-creation-ban"];

type RoleSelectorProps = {
	hideLabel?: boolean;
	loading?: boolean;
	error?: unknown;
	availableRoles?: AssignableRoles[];
	additionalImpliedRoles?: AssignableRoles[];
	selectedRoles: Set<string>;
	onChange: (roles: Set<string>) => void;
	/** When provided, called for each role. Return a tooltip string to
	 *  force-disable the role, or undefined to leave it enabled. */
	disabledReason?: (role: AssignableRoles) => string | undefined;
};

export const RoleSelector: FC<RoleSelectorProps> = ({
	hideLabel,
	loading,
	error,
	availableRoles = [],
	additionalImpliedRoles = [],
	selectedRoles,
	onChange,
	disabledReason,
}) => {
	if (loading) {
		return (
			<RoleSelectorLayout>
				<RoleSelectorSkeleton />
				<ImpliedRolesList additionalImpliedRoles={additionalImpliedRoles} />
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

	const impliedRoleNames = new Set(additionalImpliedRoles.map((r) => r.name));
	const { selectableRoles = [], advancedRoles = [] } = Object.groupBy(
		availableRoles.filter(
			(r) => r.name !== "member" && !impliedRoleNames.has(r.name),
		),
		(it) =>
			advancedRoleNames.includes(it.name) ? "advancedRoles" : "selectableRoles",
	);

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
				<RoleSelectorList
					selectableRoles={selectableRoles}
					advancedRoles={advancedRoles}
					selectedRoles={selectedRoles}
					handleToggle={handleToggle}
					disabledReason={disabledReason}
				/>
			)}

			<ImpliedRolesList additionalImpliedRoles={additionalImpliedRoles} />
		</RoleSelectorLayout>
	);
};

type RoleSelectorListProps = {
	selectableRoles: AssignableRoles[];
	advancedRoles: AssignableRoles[];
	selectedRoles: Set<string>;
	handleToggle: (roleName: string) => void;
	disabledReason?: (role: AssignableRoles) => string | undefined;
};

const RoleSelectorList: React.FC<RoleSelectorListProps> = ({
	selectableRoles,
	advancedRoles,
	selectedRoles,
	handleToggle,
	disabledReason,
}) => {
	return (
		<div className="border border-border border-solid rounded-md overflow-y-auto max-h-72 p-3 flex flex-col gap-2">
			{selectableRoles.map((role) => (
				<RoleCheckbox
					key={role.name}
					role={role}
					selected={selectedRoles.has(role.name)}
					onToggle={() => handleToggle(role.name)}
					disabledReason={disabledReason?.(role)}
				/>
			))}
			{advancedRoles.length > 0 && (
				<CollapsibleSummary label="Advanced roles" scrollIntoViewOnOpen>
					{advancedRoles.map((role) => (
						<RoleCheckbox
							key={role.name}
							role={role}
							selected={selectedRoles.has(role.name)}
							onToggle={() => handleToggle(role.name)}
							disabledReason={disabledReason?.(role)}
						/>
					))}
				</CollapsibleSummary>
			)}
		</div>
	);
};

type RoleCheckboxProps = {
	role: AssignableRoles;
	selected: boolean;
	onToggle: () => void;
	disabledReason?: string;
};

const RoleCheckbox: React.FC<RoleCheckboxProps> = ({
	role,
	selected,
	onToggle,
	disabledReason,
}) => {
	const checkboxId = useId();
	const isDisabled = !role.assignable || !!disabledReason;

	const label = (
		<label
			key={role.name}
			htmlFor={checkboxId}
			className={cn(
				"flex items-start gap-2",
				isDisabled ? "cursor-not-allowed opacity-50" : "cursor-pointer",
			)}
		>
			<Checkbox
				id={checkboxId}
				checked={selected}
				onCheckedChange={onToggle}
				disabled={isDisabled}
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

	// Disabled checkboxes apply pointer-events:none which suppresses native
	// title tooltips. Wrapping in a span keeps the tooltip reachable on hover.
	if (disabledReason) {
		return <span title={disabledReason}>{label}</span>;
	}
	return label;
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

type ImpliedRolesListProps = {
	additionalImpliedRoles: AssignableRoles[];
};

const ImpliedRolesList: React.FC<ImpliedRolesListProps> = ({
	additionalImpliedRoles,
}) => {
	return (
		<>
			<ImpliedRoleRow title="Member" description={roleDescriptions.member} />
			{additionalImpliedRoles.map((role) => (
				<ImpliedRoleRow
					key={role.name}
					title={role.display_name || role.name}
					description={roleDescriptions[role.name] ?? ""}
					caption="Sourced from organization default roles"
				/>
			))}
		</>
	);
};

type ImpliedRoleRowProps = {
	title: string;
	description: string;
	caption?: string;
};

const ImpliedRoleRow: React.FC<ImpliedRoleRowProps> = ({
	title,
	description,
	caption,
}) => {
	return (
		<div className="border-t border-border py-2 flex items-start gap-2 text-content-disabled">
			<UserIcon className="size-4 mt-1 shrink-0" />
			<div className="flex flex-col">
				<span className="text-sm font-medium">{title}</span>
				{description && <span className="text-sm">{description}</span>}
				{caption && <span className="text-xs italic">{caption}</span>}
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
