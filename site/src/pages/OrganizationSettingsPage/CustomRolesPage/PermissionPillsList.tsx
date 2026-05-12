import Stack from "@mui/material/Stack";
import type { FC } from "react";
import type { Permission } from "#/api/typesGenerated";
import { Pill } from "#/components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

function getUniqueResourceTypes(jsonObject: readonly Permission[]) {
	const resourceTypes = jsonObject.map((item) => item.resource_type);
	return [...new Set(resourceTypes)];
}

interface PermissionPillsListProps {
	permissions: readonly Permission[];
}

export const PermissionPillsList: FC<PermissionPillsListProps> = ({
	permissions,
}) => {
	const resourceTypes = getUniqueResourceTypes(permissions);

	return (
		<Stack direction="row" spacing={1}>
			{permissions.length > 0 ? (
				<PermissionsPill
					resource={resourceTypes[0]}
					permissions={permissions}
				/>
			) : (
				<p>None</p>
			)}

			{resourceTypes.length > 1 && (
				<OverflowPermissionPill
					resources={resourceTypes.slice(1)}
					permissions={permissions.slice(1)}
				/>
			)}
		</Stack>
	);
};

interface PermissionPillProps {
	resource: string;
	permissions: readonly Permission[];
}

const PermissionsPill: FC<PermissionPillProps> = ({
	resource,
	permissions,
}) => {
	const actions = permissions.filter(
		(p) => resource === p.resource_type && p.action,
	);

	return (
		<Pill type="muted" className="w-fit">
			<b>{resource}</b>:{" "}
			{actions.map((p) => `${p.negate ? "!" : ""}${p.action}`).join(", ")}
		</Pill>
	);
};

type OverflowPermissionPillProps = {
	resources: string[];
	permissions: readonly Permission[];
};

const OverflowPermissionPill: FC<OverflowPermissionPillProps> = ({
	resources,
	permissions,
}) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Pill
					type="muted"
					className="w-fit"
					data-testid="overflow-permissions-pill"
				>
					+{resources.length} more
				</Pill>
			</TooltipTrigger>

			<TooltipContent className="px-4 py-3 border-surface-quaternary">
				<ul className="flex flex-col gap-2 list-none my-0 pl-0">
					{resources.map((resource) => (
						<li key={resource}>
							<PermissionsPill resource={resource} permissions={permissions} />
						</li>
					))}
				</ul>
			</TooltipContent>
		</Tooltip>
	);
};
