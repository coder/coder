import { ChevronRightIcon, PlusIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
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

type OAuth2AppsSettingsProps = {
	apps?: TypesGen.OAuth2ProviderApp[];
	isLoading: boolean;
	error: unknown;
	canCreateApp: boolean;
	canEditSettings: boolean;
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
	canEditSettings,
	dynamicClientRegistrationEnabled,
	onDynamicClientRegistrationChange,
}) => {
	// The settings query is skipped for users without viewDeploymentConfig, so
	// an undefined value means the settings tab has nothing to show.
	const canViewSettings = dynamicClientRegistrationEnabled !== undefined;

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
						<DynamicClientRegistrationSetting
							enabled={dynamicClientRegistrationEnabled}
							canEdit={canEditSettings}
							onChange={onDynamicClientRegistrationChange}
						/>
					</TabsContent>
				)}
			</Tabs>
		</div>
	);
};

type DynamicClientRegistrationSettingProps = {
	enabled: boolean;
	canEdit: boolean;
	onChange: (enabled: boolean) => void;
};

const DynamicClientRegistrationSetting: FC<
	DynamicClientRegistrationSettingProps
> = ({ enabled, canEdit, onChange }) => {
	const headingId = useId();
	const [isEnableDialogOpen, setIsEnableDialogOpen] = useState(false);

	return (
		<>
			<section
				aria-labelledby={headingId}
				className="flex flex-row items-start justify-between gap-8"
			>
				<div className="flex flex-col gap-1 max-w-xl">
					<div className="flex flex-row items-center gap-2">
						<h3
							id={headingId}
							className="text-content-primary text-base font-semibold m-0"
						>
							Dynamic Client Registration
						</h3>
						{enabled && (
							<Badge size="sm" variant="green" className="border-0 shadow-none">
								Enabled
							</Badge>
						)}
					</div>
					<p className="text-sm text-content-secondary m-0">
						Allow OAuth2 clients to register themselves against this deployment
						without prior administrator approval (RFC 7591).
					</p>
				</div>

				{enabled ? (
					<Button
						variant="outline"
						disabled={!canEdit}
						onClick={() => onChange(false)}
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
						<DialogActions
							confirmVariant="destructive"
							onConfirm={() => {
								setIsEnableDialogOpen(false);
								onChange(true);
							}}
							onCancel={() => setIsEnableDialogOpen(false)}
						/>
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
