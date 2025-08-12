import Button from "@mui/material/Button";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { ClientCredentialsAppRow } from "components/OAuth2/ClientCredentialsAppRow";
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import { PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

type OAuth2ProviderPageViewProps = {
	isLoading: boolean;
	error: unknown;
	authorizedApps?: TypesGen.OAuth2ProviderApp[];
	ownedApps?: TypesGen.OAuth2ProviderApp[];
	revoke: (app: TypesGen.OAuth2ProviderApp) => void;
	onManageOwnedApp: (app: TypesGen.OAuth2ProviderApp) => void;
	onDeleteOwnedApp: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2ProviderPageView: FC<OAuth2ProviderPageViewProps> = ({
	isLoading,
	error,
	authorizedApps,
	ownedApps,
	revoke,
	onManageOwnedApp,
	onDeleteOwnedApp,
}) => {
	return (
		<Stack spacing={4}>
			{error ? <ErrorAlert error={error} /> : null}

			{/* My Applications Section */}
			<div>
				<div className="mb-4 flex items-center justify-between">
					<div>
						<h3 className="text-lg font-medium text-content-primary">
							My Applications
						</h3>
						<p className="text-sm text-content-secondary">
							OAuth2 applications you've created for API access
						</p>
					</div>
					<Button
						startIcon={<PlusIcon className="size-icon-sm" />}
						component={RouterLink}
						to="new"
						variant="contained"
					>
						Create Application
					</Button>
				</div>

				<TableContainer>
					<Table>
						<TableHead>
							<TableRow>
								<TableCell width="60%">Application</TableCell>
								<TableCell width="20%">Type</TableCell>
								<TableCell width="20%">Actions</TableCell>
							</TableRow>
						</TableHead>
						<TableBody>
							{isLoading && <TableLoader />}
							{ownedApps?.map((app) => (
								<ClientCredentialsAppRow
									key={app.id}
									app={app}
									onManage={onManageOwnedApp}
									onDelete={onDeleteOwnedApp}
								/>
							))}
							{ownedApps?.length === 0 && !isLoading && (
								<TableRow>
									<TableCell colSpan={3}>
										<div css={{ textAlign: "center", padding: "32px 16px" }}>
											<p className="text-content-secondary">
												No applications created yet.
											</p>
											<p className="text-sm text-content-secondary mt-1">
												Create your first OAuth2 application to start using the
												Coder API.
											</p>
										</div>
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</TableContainer>
			</div>

			{/* Authorized Applications Section */}
			<div>
				<div className="mb-4">
					<h3 className="text-lg font-medium text-content-primary">
						Authorized Applications
					</h3>
					<p className="text-sm text-content-secondary">
						OAuth2 applications you've granted access to your account
					</p>
				</div>

				<TableContainer>
					<Table>
						<TableHead>
							<TableRow>
								<TableCell width="100%">Name</TableCell>
								<TableCell width="1%" />
							</TableRow>
						</TableHead>
						<TableBody>
							{isLoading && <TableLoader />}
							{authorizedApps?.map((app) => (
								<OAuth2AppRow key={app.id} app={app} revoke={revoke} />
							))}
							{authorizedApps?.length === 0 && !isLoading && (
								<TableRow>
									<TableCell colSpan={2}>
										<div css={{ textAlign: "center", padding: "32px 16px" }}>
											<p className="text-content-secondary">
												No OAuth2 applications have been authorized.
											</p>
										</div>
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</TableContainer>
			</div>
		</Stack>
	);
};

type OAuth2AppRowProps = {
	app: TypesGen.OAuth2ProviderApp;
	revoke: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2AppRow: FC<OAuth2AppRowProps> = ({ app, revoke }) => {
	return (
		<TableRow key={app.id} data-testid={`app-${app.id}`}>
			<TableCell>
				<Stack direction="row" spacing={1} alignItems="center">
					<Avatar variant="icon" src={app.icon} fallback={app.name} />
					<span className="font-semibold">{app.name}</span>
				</Stack>
			</TableCell>

			<TableCell>
				<Button
					variant="contained"
					size="small"
					color="error"
					onClick={() => revoke(app)}
				>
					Revoke&hellip;
				</Button>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2ProviderPageView;
