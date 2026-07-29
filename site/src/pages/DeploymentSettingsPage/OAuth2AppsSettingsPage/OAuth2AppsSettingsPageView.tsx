import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
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
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { DynamicClientRegistrationSetting } from "./DynamicClientRegistrationSetting";

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoading: boolean;
	error: unknown;
	canCreateApp: boolean;
	canViewSettings: boolean;
	canEditSettings: boolean;
	isLoadingSettings: boolean;
	isUpdatingSettings: boolean;
	dynamicClientRegistrationEnabled: boolean | undefined;
	onDynamicClientRegistrationChange: (enabled: boolean) => void;
};

const AddApplicationButton: FC = () => (
	<Button variant="outline" asChild>
		<Link to="/deployment/oauth2-provider/apps/add">
			<PlusIcon />
			<span>Add application</span>
		</Link>
	</Button>
);

const OAuth2AppsSettingsPageView: FC<OAuth2AppsSettingsProps> = ({
	apps,
	isLoading,
	error,
	canCreateApp,
	canViewSettings,
	canEditSettings,
	isLoadingSettings,
	isUpdatingSettings,
	dynamicClientRegistrationEnabled,
	onDynamicClientRegistrationChange,
}) => {
	return (
		<div>
			<SettingsHeader
				actions={canCreateApp ? <AddApplicationButton /> : undefined}
			>
				<SettingsHeaderTitle>OAuth2 applications</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure applications to use Coder as an OAuth2 provider.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}

			<Tabs defaultValue="applications">
				<TabsList>
					<TabsTrigger value="applications">Applications</TabsTrigger>
					{canViewSettings && (
						<TabsTrigger value="settings">Settings</TabsTrigger>
					)}
				</TabsList>

				<TabsContent value="applications" className="pt-6">
					<Table className="table-fixed" aria-label="OAuth2 applications">
						<TableHeader>
							<TableRow>
								<TableHead className="w-1/3">Name</TableHead>
								<TableHead className="w-1/3">Callback URL</TableHead>
								<TableHead className="w-12">
									<span className="sr-only">Open</span>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody size="lg">
							{isLoading ? (
								<TableLoader />
							) : !error && (!apps || apps.length === 0) ? (
								<TableEmpty
									message="No OAuth2 applications configured"
									description="Add an application to use Coder as an OAuth2 provider."
									cta={canCreateApp ? <AddApplicationButton /> : undefined}
								/>
							) : (
								apps?.map((app) => <OAuth2AppRow key={app.id} app={app} />)
							)}
						</TableBody>
					</Table>
				</TabsContent>

				{canViewSettings && (
					<TabsContent value="settings" className="pt-6">
						{isLoadingSettings && <Loader label="Loading settings" />}
						{dynamicClientRegistrationEnabled !== undefined && (
							<DynamicClientRegistrationSetting
								enabled={dynamicClientRegistrationEnabled}
								canEdit={canEditSettings}
								isUpdating={isUpdatingSettings}
								onChange={onDynamicClientRegistrationChange}
							/>
						)}
					</TabsContent>
				)}
			</Tabs>
		</div>
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
