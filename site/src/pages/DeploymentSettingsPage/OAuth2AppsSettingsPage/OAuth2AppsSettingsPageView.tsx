import { ChevronRightIcon, PlusIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
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
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";

// Number of apps rendered per page. Matches other paginated tables
// (e.g. the Workspaces table). The pagination controls only render
// when there is more than one page worth of apps.
const PAGE_SIZE = 25;

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
	const [page, setPage] = useState(1);

	const totalRecords = apps?.length ?? 0;
	const totalPages = Math.max(1, Math.ceil(totalRecords / PAGE_SIZE));
	// If the current page exceeds the number of available pages (e.g.
	// after an app was deleted), fall back to the last valid page.
	const currentPage = Math.min(page, totalPages);

	const pagedApps = useMemo(() => {
		if (!apps) {
			return undefined;
		}
		const start = (currentPage - 1) * PAGE_SIZE;
		return apps.slice(start, start + PAGE_SIZE);
	}, [apps, currentPage]);

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

			{error && <ErrorAlert error={error} />}

			<Table className="mt-8">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading && <TableLoader />}
					{pagedApps?.map((app) => (
						<OAuth2AppRow key={app.id} app={app} />
					))}
					{apps?.length === 0 && (
						<TableRow>
							<TableCell colSpan={999}>
								<div className="text-center">
									No OAuth2 applications have been configured.
								</div>
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>

			<div className="pt-4">
				<PaginationWidgetBase
					totalRecords={totalRecords}
					pageSize={PAGE_SIZE}
					currentPage={currentPage}
					onPageChange={setPage}
				/>
			</div>
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
		<TableRow key={app.id} data-testid={`app-${app.id}`} {...clickableProps}>
			<TableCell>
				<AvatarData
					avatar={<Avatar variant="icon" src={app.icon} fallback={app.name} />}
					title={app.name}
				/>
			</TableCell>

			<TableCell>
				<div className="flex pl-4">
					<ChevronRightIcon className="size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2AppsSettingsPageView;
