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
import { useSearchParamsKey } from "#/hooks/useSearchParamsKey";
import { DynamicClientRegistrationSetting } from "./DynamicClientRegistrationSetting";

/**
 * Absent when the viewer cannot read deployment config, so "cannot view" is
 * the shape of the prop rather than a flag the caller has to keep consistent
 * with the five values beside it.
 */
type SettingsTab = {
	canEdit: boolean;
	isLoading: boolean;
	isUpdating: boolean;
	error: unknown;
	dynamicClientRegistrationEnabled: boolean | undefined;
	onDynamicClientRegistrationChange: (enabled: boolean) => void;
};

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoading: boolean;
	error: unknown;
	canCreateApp: boolean;
	settings?: SettingsTab;
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
	settings,
}) => {
	const tabState = useSearchParamsKey({
		key: "tab",
		defaultValue: "applications",
	});
	// Unknown values, and the settings tab for users who cannot view it, fall
	// back to the applications tab rather than selecting nothing.
	const activeTab =
		tabState.value === "settings" && settings ? "settings" : "applications";

	return (
		<div>
			{/*
			 * The header sits outside the tabs, so a tab-specific action here would
			 * promise to act on content it navigates away from. Adding an
			 * application belongs to the applications tab alone.
			 */}
			<SettingsHeader
				actions={
					canCreateApp && activeTab === "applications" ? (
						<AddApplicationButton />
					) : undefined
				}
			>
				<SettingsHeaderTitle>OAuth2 applications</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Register applications to use Coder as an OAuth2 provider, and
					configure how this deployment behaves as one.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}

			<Tabs value={activeTab} onValueChange={tabState.setValue}>
				<TabsList>
					<TabsTrigger value="applications">Applications</TabsTrigger>
					{settings && <TabsTrigger value="settings">Settings</TabsTrigger>}
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

				{settings && (
					<TabsContent value="settings" className="pt-6">
						{settings.isLoading ? (
							<Loader label="Loading settings" />
						) : (
							<div className="flex flex-col gap-4">
								{Boolean(settings.error) && (
									<ErrorAlert error={settings.error} />
								)}
								{settings.dynamicClientRegistrationEnabled !== undefined && (
									<DynamicClientRegistrationSetting
										enabled={settings.dynamicClientRegistrationEnabled}
										canEdit={settings.canEdit}
										isUpdating={settings.isUpdating}
										onChange={settings.onDynamicClientRegistrationChange}
									/>
								)}
								{/*
								 * The value is optional on the wire, so a response that omits
								 * it would otherwise leave this tab blank with nothing to
								 * explain why.
								 */}
								{!settings.error &&
									settings.dynamicClientRegistrationEnabled === undefined && (
										<p className="text-sm text-content-secondary m-0">
											Settings are unavailable.
										</p>
									)}
							</div>
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
