import { ChevronRightIcon, PlusIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { Link, useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Label } from "#/components/Label/Label";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
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
	const dcrSwitchId = useId();
	const [isEnableDcrDialogOpen, setIsEnableDcrDialogOpen] = useState(false);

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

			{dynamicClientRegistrationEnabled !== undefined && (
				<div className="flex flex-row items-center gap-3 mb-6">
					<Switch
						id={dcrSwitchId}
						checked={dynamicClientRegistrationEnabled}
						disabled={!canEditSettings}
						onCheckedChange={(checked) => {
							if (checked) {
								setIsEnableDcrDialogOpen(true);
							} else {
								onDynamicClientRegistrationChange(false);
							}
						}}
					/>
					<Label htmlFor={dcrSwitchId}>Dynamic Client Registration</Label>
				</div>
			)}

			<Dialog
				open={isEnableDcrDialogOpen}
				onOpenChange={setIsEnableDcrDialogOpen}
			>
				<DialogContent className="flex flex-col gap-12 max-w-lg">
					<DialogHeader className="flex flex-col gap-4">
						<DialogTitle>Enable Dynamic Client Registration</DialogTitle>
						<DialogDescription>
							Warning: Any OAuth2 client will be able to register itself against
							this deployment (RFC 7591) without prior approval from an
							administrator. Only enable this if you intend to support
							self-service client registration.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter className="flex flex-row">
						<Button
							variant="outline"
							onClick={() => setIsEnableDcrDialogOpen(false)}
						>
							Cancel
						</Button>
						<Button
							onClick={() => {
								setIsEnableDcrDialogOpen(false);
								onDynamicClientRegistrationChange(true);
							}}
						>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

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
