import { useTheme } from "@emotion/react";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
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
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useNavigate } from "react-router";

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoading: boolean;
	error: unknown;
};

const OAuth2AppsSettingsPageView: FC<OAuth2AppsSettingsProps> = ({
	apps,
	isLoading,
	error,
}) => {
	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle>OAuth2 Applications</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Configure applications to use Coder as an OAuth2 provider.
						</SettingsHeaderDescription>
					</SettingsHeader>
				</div>

				<Button variant="outline" asChild>
					<Link to="/deployment/oauth2-provider/apps/add">
						<PlusIcon />
						Add application
					</Link>
				</Button>
			</Stack>

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
					{apps?.map((app) => (
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
		</>
	);
};

type OAuth2AppRowProps = {
	app: TypesGen.OAuth2ProviderApp;
};

const OAuth2AppRow: FC<OAuth2AppRowProps> = ({ app }) => {
	const _theme = useTheme();
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
