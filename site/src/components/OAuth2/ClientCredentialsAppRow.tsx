import Button from "@mui/material/Button";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import type { FC } from "react";

type ClientCredentialsAppRowProps = {
	app: TypesGen.OAuth2ProviderApp;
	onManage: (app: TypesGen.OAuth2ProviderApp) => void;
	onDelete: (app: TypesGen.OAuth2ProviderApp) => void;
};

export const ClientCredentialsAppRow: FC<ClientCredentialsAppRowProps> = ({
	app,
	onManage,
	onDelete,
}) => {
	const clickableProps = useClickableTableRow({
		onClick: () => onManage(app),
	});

	return (
		<TableRow
			key={app.id}
			data-testid={`owned-app-${app.id}`}
			{...clickableProps}
		>
			<TableCell>
				<Stack direction="row" spacing={1} alignItems="center">
					<Avatar variant="icon" src={app.icon} fallback={app.name} />
					<div>
						<span className="font-semibold">{app.name}</span>
						<div className="text-xs text-content-secondary">
							Created {dayjs(app.created_at).format("MMM D, YYYY")}
						</div>
					</div>
				</Stack>
			</TableCell>

			<TableCell>
				<span className="rounded bg-surface-secondary px-2 py-1 text-xs text-content-secondary">
					Client Credentials
				</span>
			</TableCell>

			<TableCell
				onClick={(e) => {
					e.stopPropagation(); // Prevent row click
				}}
			>
				<Stack direction="row" spacing={1}>
					<Button variant="outlined" size="small" onClick={() => onManage(app)}>
						Manage
					</Button>
					<Button
						variant="contained"
						size="small"
						color="error"
						onClick={() => onDelete(app)}
					>
						Delete&hellip;
					</Button>
				</Stack>
			</TableCell>
		</TableRow>
	);
};
