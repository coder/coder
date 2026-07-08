import { CircleDollarSignIcon } from "lucide-react";
import type { FC } from "react";
import { useId } from "react";
import type { AssignableRoles } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";

const workspaceAccessRoleName = "organization-workspace-access";
const aiGatewayAccessRoleName = "organization-ai-gateway-access";

type DefaultRolesPreset = "workspace" | "gateway" | "custom";

const rolesMatch = (
	roles: readonly string[],
	expected: readonly string[],
): boolean => {
	return (
		roles.length === expected.length &&
		expected.every((role) => roles.includes(role))
	);
};

const presetForRoles = (roles: readonly string[]): DefaultRolesPreset => {
	if (rolesMatch(roles, [aiGatewayAccessRoleName])) {
		return "gateway";
	}
	if (rolesMatch(roles, [workspaceAccessRoleName, aiGatewayAccessRoleName])) {
		return "workspace";
	}
	return "custom";
};

interface DefaultRolesPresetCardsProps {
	roles: readonly string[];
	availableRoles?: AssignableRoles[];
	disabled: boolean;
	onSelectRoles: (roles: string[]) => void;
	onSelectCustom: () => void;
}

export const DefaultRolesPresetCards: FC<DefaultRolesPresetCardsProps> = ({
	roles,
	availableRoles,
	disabled,
	onSelectRoles,
	onSelectCustom,
}) => {
	const preset = presetForRoles(roles);

	return (
		<div
			role="radiogroup"
			aria-label="Default roles preset"
			className="grid grid-cols-1 gap-4 md:grid-cols-3"
		>
			<PresetCard
				name="Gateway"
				description="Members can only route AI traffic through the AI Gateway, with their requests recorded under their own identity."
				note={{
					text: "Gateway members do not cost license seats.",
					tone: "free",
				}}
				selected={preset === "gateway"}
				disabled={disabled}
				onSelect={() => {
					if (preset !== "gateway") {
						onSelectRoles([aiGatewayAccessRoleName]);
					}
				}}
			/>
			<PresetCard
				name="Workspace"
				description="Members receive full workspace permissions (create, connect to, start, and stop workspaces) and can use the AI Gateway."
				note={{
					text: "Every user added to this organization counts as a license seat.",
					tone: "cost",
				}}
				selected={preset === "workspace"}
				disabled={disabled}
				onSelect={() => {
					if (preset !== "workspace") {
						onSelectRoles([workspaceAccessRoleName, aiGatewayAccessRoleName]);
					}
				}}
			/>
			<PresetCard
				name="Custom"
				description="Choose a custom set of default roles for members of this organization."
				selected={preset === "custom"}
				disabled={disabled}
				onSelect={onSelectCustom}
			>
				{preset === "custom" && (
					<ul className="list-disc pl-5 m-0 mt-2 flex flex-col gap-1 text-sm text-content-primary">
						{roles.map((name) => (
							<li key={name}>
								{availableRoles?.find((role) => role.name === name)
									?.display_name || name}
							</li>
						))}
					</ul>
				)}
			</PresetCard>
		</div>
	);
};

interface PresetCardNote {
	text: string;
	// cost renders with warning emphasis, free with success emphasis.
	tone: "cost" | "free";
}

interface PresetCardProps {
	name: string;
	description: string;
	note?: PresetCardNote;
	selected: boolean;
	disabled: boolean;
	onSelect: () => void;
	children?: React.ReactNode;
}

const PresetCard: FC<PresetCardProps> = ({
	name,
	description,
	note,
	selected,
	disabled,
	onSelect,
	children,
}) => {
	const nameId = useId();

	return (
		<div
			role="radio"
			aria-checked={selected}
			aria-disabled={disabled}
			aria-labelledby={nameId}
			tabIndex={disabled ? -1 : 0}
			className={cn(
				"flex flex-col p-4 rounded",
				"bg-surface-secondary border border-solid",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-border-primary",
				selected ? "border-border-pending" : "border-border",
				disabled ? "opacity-60" : "cursor-pointer",
			)}
			onClick={() => {
				if (!disabled) {
					onSelect();
				}
			}}
			onKeyDown={(e) => {
				if (!disabled && (e.key === "Enter" || e.key === " ")) {
					e.preventDefault();
					onSelect();
				}
			}}
		>
			<div className="flex items-start justify-between mb-2">
				<h3
					id={nameId}
					className="m-0 text-md font-semibold text-content-primary"
				>
					{name}
				</h3>
				<div
					aria-hidden="true"
					className={cn(
						"flex items-center justify-center size-4 rounded-full border border-solid mt-0.5 shrink-0",
						selected ? "border-content-primary" : "border-content-secondary",
					)}
				>
					{selected && (
						<div className="size-2 rounded-full bg-content-primary" />
					)}
				</div>
			</div>
			<p className="m-0 text-sm font-normal text-content-secondary">
				{description}
			</p>
			{note && (
				<p
					className={cn(
						"m-0 mt-2 flex items-start gap-1.5 text-sm font-semibold",
						note.tone === "cost"
							? "text-content-warning"
							: "text-content-success",
					)}
				>
					<CircleDollarSignIcon
						aria-hidden="true"
						className="size-4 shrink-0 mt-0.5"
					/>
					{note.text}
				</p>
			)}
			{children}
		</div>
	);
};
