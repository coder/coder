import { ChevronRightIcon, PlusIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
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
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
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
	// Visual-mock props for the Settings tab. Wired up to local state so the
	// design can be reviewed before the real API from #27480 lands.
	canEditSettings?: boolean;
	dynamicClientRegistrationEnabled?: boolean;
	onDynamicClientRegistrationChange?: (enabled: boolean) => void;
};

const OAuth2AppsSettingsPageView: FC<OAuth2AppsSettingsProps> = ({
	apps,
	isLoading,
	error,
	canCreateApp,
	canEditSettings = true,
	dynamicClientRegistrationEnabled = false,
	onDynamicClientRegistrationChange,
}) => {
	const [page, setPage] = useState(1);
	const [activeTab, setActiveTab] = useState<"applications" | "settings">(
		"applications",
	);

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

			<Tabs
				value={activeTab}
				onValueChange={(v) => setActiveTab(v as "applications" | "settings")}
				className="mt-6"
			>
				<TabsList>
					<TabsTrigger value="applications">Applications</TabsTrigger>
					<TabsTrigger value="settings">Settings</TabsTrigger>
				</TabsList>

				<TabsContent value="applications" className="pt-6">
					<Table>
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
				</TabsContent>

				<TabsContent value="settings" className="pt-6">
					<DynamicClientRegistrationSetting
						enabled={dynamicClientRegistrationEnabled}
						canEdit={canEditSettings}
						onChange={onDynamicClientRegistrationChange}
					/>
				</TabsContent>
			</Tabs>
		</>
	);
};

type DynamicClientRegistrationSettingProps = {
	enabled: boolean;
	canEdit: boolean;
	onChange?: (enabled: boolean) => void;
};

const DynamicClientRegistrationSetting: FC<
	DynamicClientRegistrationSettingProps
> = ({ enabled, canEdit, onChange }) => {
	const [localEnabled, setLocalEnabled] = useState(enabled);
	const [isEnableDialogOpen, setIsEnableDialogOpen] = useState(false);

	const isEnabled = onChange ? enabled : localEnabled;

	const handleChange = (next: boolean) => {
		if (onChange) {
			onChange(next);
		} else {
			setLocalEnabled(next);
		}
	};

	return (
		<>
			<section>
				<div className="flex flex-row items-start justify-between gap-8">
					<div className="flex flex-col gap-1 max-w-xl">
						<div className="flex flex-row items-center gap-2">
							<h3 className="text-content-primary text-base font-semibold m-0">
								Dynamic Client Registration
							</h3>
							{isEnabled && (
								<Badge
									size="sm"
									variant="green"
									className="border-0 shadow-none"
								>
									Enabled
								</Badge>
							)}
						</div>
						<p className="text-sm text-content-secondary m-0">
							Allow OAuth2 clients to register themselves against this
							deployment without prior administrator approval (RFC 7591).
						</p>
					</div>

					{isEnabled ? (
						<Button
							variant="outline"
							disabled={!canEdit}
							onClick={() => handleChange(false)}
						>
							Disable
						</Button>
					) : (
						<Button
							disabled={!canEdit}
							onClick={() => setIsEnableDialogOpen(true)}
						>
							Enable
						</Button>
					)}
				</div>
			</section>

			<Dialog open={isEnableDialogOpen} onOpenChange={setIsEnableDialogOpen}>
				<DialogContent variant="destructive" className="max-w-xl">
					<DialogHeader>
						<DialogTitle>Enable Dynamic Client Registration?</DialogTitle>
						<DialogDescription>
							Only enable Dynamic Client Registration if you intend to support
							self-service OAuth2 client registration.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setIsEnableDialogOpen(false)}
						>
							Cancel
						</Button>
						<Button
							variant="destructive"
							onClick={() => {
								setIsEnableDialogOpen(false);
								handleChange(true);
							}}
						>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
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
