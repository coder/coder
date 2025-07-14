import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { ClientCredentialsAppRow } from "components/OAuth2/ClientCredentialsAppRow";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
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

				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-[60%]">Application</TableHead>
							<TableHead className="w-[20%]">Type</TableHead>
							<TableHead className="w-[20%]">Actions</TableHead>
						</TableRow>
					</TableHeader>
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
									<div className="px-4 py-8 text-center text-content-secondary">
										<p>No applications created yet.</p>
										<p className="mt-1 text-sm">
											Create your first OAuth2 application to start using the
											Coder API.
										</p>
									</div>
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			</div>

			<div>
				<div className="mb-4">
					<h3 className="text-lg font-medium text-content-primary">
						Authorized Applications
					</h3>
					<p className="text-sm text-content-secondary">
						OAuth2 applications you've granted access to your account
					</p>
				</div>

				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead className="w-[1%]" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading && <TableLoader />}
						{authorizedApps?.map((app) => (
							<OAuth2AppRow key={app.id} app={app} revoke={revoke} />
						))}
						{authorizedApps?.length === 0 && !isLoading && (
							<TableRow>
								<TableCell colSpan={2}>
									<div className="px-4 py-8 text-center text-content-secondary">
										No OAuth2 applications have been authorized.
									</div>
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
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
				<Button size="sm" variant="destructive" onClick={() => revoke(app)}>
					Revoke&hellip;
				</Button>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2ProviderPageView;
