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
	SettingsHeaderDocsLink,
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
import { docs } from "#/utils/docs";
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
	/**
	 * Kept apart because the two need opposite treatment. A load failure means
	 * there is no value to act on, so the control must not render. An update
	 * failure leaves the value valid, so the control stays and the admin can
	 * retry. Merging them also let the older one hide the newer.
	 */
	loadError: unknown;
	updateError: unknown;
	/**
	 * Both terminal states below are dead ends without it: `retry: false` and
	 * `refetchOnWindowFocus: false` are set globally, the query outlives a tab
	 * switch, and the control that would trigger an invalidation is exactly what
	 * is not rendered.
	 */
	onRetry: () => void;
	// Stays optional: an offline query is `fetchStatus: "paused"`, so `isLoading`
	// is false with no data and no error.
	dynamicClientRegistrationEnabled: boolean | undefined;
	onDynamicClientRegistrationChange: (enabled: boolean) => void;
};

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoadingApps: boolean;
	appsError: unknown;
	canCreateApp: boolean;
	settings?: SettingsTab;
};

/**
 * Four states, decided in order. Whether the control renders depends on whether
 * there is a value to act on, never on which error happens to be set, and the
 * update error wins the alert because it reports the action the admin just took.
 */
const SettingsTabBody: FC<{ settings: SettingsTab }> = ({ settings }) => {
	if (settings.isLoading) {
		return <Loader label="Loading settings" />;
	}

	if (settings.dynamicClientRegistrationEnabled === undefined) {
		return (
			<div className="flex flex-col items-start gap-4">
				{settings.loadError ? (
					<ErrorAlert error={settings.loadError} />
				) : (
					<p className="text-sm text-content-secondary m-0">
						Coder did not return a value for Dynamic Client Registration. This
						can happen while the browser is offline. Retry, or check the setting
						with <code className="text-xs">coder oauth2-provider dcr</code>.
					</p>
				)}
				<Button variant="outline" size="sm" onClick={settings.onRetry}>
					Retry
				</Button>
			</div>
		);
	}

	const alertError = settings.updateError ?? settings.loadError;
	return (
		<div className="flex flex-col gap-4">
			{Boolean(alertError) && <ErrorAlert error={alertError} />}
			<DynamicClientRegistrationSetting
				enabled={settings.dynamicClientRegistrationEnabled}
				canEdit={settings.canEdit}
				isUpdating={settings.isUpdating}
				onChange={settings.onDynamicClientRegistrationChange}
			/>
		</div>
	);
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
	isLoadingApps,
	appsError,
	canCreateApp,
	settings,
}) => {
	const tabState = useSearchParamsKey({
		key: "tab",
		defaultValue: "applications",
	});
	// A value matching no trigger would leave no tab selected.
	const activeTab =
		tabState.value === "settings" && settings ? "settings" : "applications";

	return (
		<div>
			{/*
			 * The header sits outside the tabs, so a tab-specific action here would
			 * promise to act on content it navigates away from. The docs link is
			 * tab-agnostic, so it stays on both.
			 */}
			<SettingsHeader
				actions={
					<>
						{canCreateApp && activeTab === "applications" && (
							<AddApplicationButton />
						)}
						<SettingsHeaderDocsLink
							href={docs(
								"/admin/integrations/oauth2-provider#dynamic-client-registration",
							)}
						/>
					</>
				}
			>
				<SettingsHeaderTitle>OAuth2 applications</SettingsHeaderTitle>
				{/*
				 * The second clause describes the settings tab, which is absent for a
				 * viewer who cannot read deployment config. Promising it to someone
				 * with no control for it on the page sends them looking for one.
				 */}
				<SettingsHeaderDescription>
					Register applications to use Coder as an OAuth2 provider
					{settings && ", and configure how this deployment behaves as one"}.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<Tabs value={activeTab} onValueChange={tabState.setValue}>
				<TabsList>
					<TabsTrigger value="applications">Applications</TabsTrigger>
					{settings && <TabsTrigger value="settings">Settings</TabsTrigger>}
				</TabsList>

				<TabsContent value="applications" className="pt-6">
					{/*
					 * Inside the tab, for the same reason the settings error is: an
					 * error belongs with the content it describes. Outside, an apps
					 * failure sat above the settings panel and read as that panel's.
					 */}
					{Boolean(appsError) && (
						<div className="mb-4">
							<ErrorAlert error={appsError} />
						</div>
					)}
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
							{isLoadingApps ? (
								<TableLoader />
							) : !appsError && (!apps || apps.length === 0) ? (
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
						<SettingsTabBody settings={settings} />
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
						className="size-icon-sm text-content-secondary flex-shrink-0"
					/>
				</div>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2AppsSettingsPageView;
