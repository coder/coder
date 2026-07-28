import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoading: boolean;
	error: unknown;
	canCreateApp: boolean;
};

const OAuth2AppsSettingsPageView: FC<OAuth2AppsSettingsProps> = ({
	apps,
	isLoading,
	error,
	canCreateApp,
}) => {
	return (
		<>
			<div className="flex flex-row gap-4 items-baseline justify-between">
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle>OAuth2 Applications</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Configure applications to use Coder as an OAuth2 provider.
						</SettingsHeaderDescription>
					</SettingsHeader>
				</div>

				{canCreateApp && (
					<Button variant="outline" asChild>
						<Link to="/deployment/oauth2-provider/apps/add">
							<PlusIcon />
							Add application
						</Link>
					</Button>
				)}
			</div>

			{error ? <ErrorAlert error={error} /> : undefined}

			<Table className="table-fixed" aria-label="OAuth2 applications">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/2">Callback URL</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">
					{isLoading && <TableLoader />}
					{!isLoading && (!apps || apps.length === 0) && (
						<TableEmpty
							message="No OAuth2 applications have been configured."
							cta={
								canCreateApp ? (
									<Button variant="outline" asChild>
										<Link to="/deployment/oauth2-provider/apps/add">
											<PlusIcon />
											Add application
										</Link>
									</Button>
								) : undefined
							}
						/>
					)}
					{!isLoading &&
						apps?.map((app) => <OAuth2AppRow key={app.id} app={app} />)}
				</TableBody>
			</Table>
		</>
	);
};

type OAuth2AppRowProps = {
	app: TypesGen.OAuth2ProviderApp;
};

const OAuth2AppRow: FC<OAuth2AppRowProps> = ({ app }) => {
	const navigate = useNavigate();
	const clickableProps = useClickableTableRow({
		onClick: () => navigate(`/deployment/oauth2-provider/apps/${app.id}`),
	});

	return (
		<TableRow data-testid={`app-${app.id}`} {...clickableProps}>
			<TableCell className="min-w-0 px-4 py-3">
				<AvatarData
					avatar={
						<Avatar
							variant="icon"
							size="lg"
							src={app.icon}
							fallback={app.name}
						/>
					}
					title={app.name}
				/>
			</TableCell>
			<TableCell className="min-w-0">
				<span
					className="block truncate text-content-secondary"
					title={app.callback_url}
				>
					{app.callback_url}
				</span>
			</TableCell>
			<TableCell className="w-10 text-center">
				<div className="flex justify-end items-center pr-4">
					<ChevronRightIcon
						aria-hidden
						className="size-icon-md text-content-primary flex-shrink-0"
					/>
				</div>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2AppsSettingsPageView;
