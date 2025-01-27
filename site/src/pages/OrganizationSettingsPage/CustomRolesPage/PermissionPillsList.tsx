import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Stack from "@mui/material/Stack";
import type { Permission } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import type { FC } from "react";

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
		<Pill css={styles.permissionPill}>
			<b>{resource}</b>: {actions.map((p) => p.action).join(", ")}
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
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					css={{
						backgroundColor: theme.palette.background.paper,
						borderColor: theme.palette.divider,
					}}
					data-testid="overflow-permissions-pill"
				>
					+{resources.length} more
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				disableRestoreFocus
				disableScrollLock
				css={{
					".MuiPaper-root": {
						display: "flex",
						flexFlow: "column wrap",
						columnGap: 8,
						rowGap: 12,
						padding: "12px 16px",
						alignContent: "space-around",
						minWidth: "auto",
						backgroundColor: theme.palette.background.default,
					},
				}}
				anchorOrigin={{
					vertical: -4,
					horizontal: "center",
				}}
				transformOrigin={{
					vertical: "bottom",
					horizontal: "center",
				}}
			>
				{resources.map((resource) => (
					<PermissionsPill
						key={resource}
						resource={resource}
						permissions={permissions}
					/>
				))}
			</PopoverContent>
		</Popover>
	);
};

const styles = {
	permissionPill: (theme) => ({
		backgroundColor: theme.experimental.pillDefault.background,
		borderColor: theme.experimental.pillDefault.outline,
		color: theme.experimental.pillDefault.text,
		width: "fit-content",
	}),
} satisfies Record<string, Interpolation<Theme>>;
