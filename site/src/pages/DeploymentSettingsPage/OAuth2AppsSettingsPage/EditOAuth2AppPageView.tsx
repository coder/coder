import { isAxiosError } from "axios";
import { ArrowLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	Link,
	Navigate,
	useNavigate,
	useParams,
	useSearchParams,
} from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import * as oauth2 from "#/api/queries/oauth2";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { Loader } from "#/components/Loader/Loader";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
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
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { createDayString } from "#/utils/createDayString";
import { pageTitle } from "#/utils/page";
import { OAuth2AppForm } from "./OAuth2AppForm";

const BACK_HREF = "/deployment/oauth2-provider/apps";

export const EditOAuth2AppPageView: FC = () => {
	const { appId } = useParams<{ appId: string }>();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();

	const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
	const [iconOverride, setIconOverride] = useState<string>();
	// When a new secret is created it is returned with the full secret. This is
	// the only time it will be visible. The secret list only returns a truncated
	// version. Once the user acknowledges the secret we clear it from state.
	const [fullNewSecret, setFullNewSecret] =
		useState<TypesGen.OAuth2ProviderAppSecretFull>();

	const appQuery = useQuery({
		...oauth2.getApp(appId ?? ""),
		enabled: Boolean(appId),
	});
	const secretsQuery = useQuery({
		...oauth2.getAppSecrets(appId ?? ""),
		enabled: Boolean(appId) && permissions.viewOAuth2AppSecrets,
	});

	const putAppMutation = useMutation(oauth2.putApp(queryClient));
	const deleteAppMutation = useMutation(oauth2.deleteApp(queryClient));
	const postSecretMutation = useMutation(oauth2.postAppSecret(queryClient));
	const deleteSecretMutation = useMutation(oauth2.deleteAppSecret(queryClient));

	const app = appQuery.data;
	const title = (
		<title>{pageTitle(app?.name ?? "Loading...", "OAuth2 applications")}</title>
	);

	if (!appId) {
		return <Navigate to={BACK_HREF} replace />;
	}

	if (appQuery.isLoading) {
		return (
			<>
				{title}
				<Loader fullscreen />
			</>
		);
	}

	if (appQuery.isError) {
		const status = isAxiosError(appQuery.error)
			? appQuery.error.response?.status
			: undefined;
		if (status === 404) {
			return <Navigate to={BACK_HREF} replace />;
		}
		return (
			<>
				{title}
				<div className="flex flex-col gap-4">
					<p className="text-content-secondary m-0">
						{getErrorMessage(
							appQuery.error,
							"Failed to load OAuth2 application.",
						)}
					</p>
					<Button variant="subtle" asChild className="-ml-3">
						<Link to={BACK_HREF}>
							<ArrowLeftIcon />
							<span>Back to applications</span>
						</Link>
					</Button>
				</div>
			</>
		);
	}

	if (!app) {
		return <Navigate to={BACK_HREF} replace />;
	}

	const canEditApp = permissions.editOAuth2App;
	const canDeleteApp = permissions.deleteOAuth2App;
	const canViewAppSecrets = permissions.viewOAuth2AppSecrets;
	const isMutating =
		putAppMutation.isPending ||
		deleteAppMutation.isPending ||
		postSecretMutation.isPending ||
		deleteSecretMutation.isPending;

	return (
		<>
			{title}

			<div className="flex justify-between items-center">
				<Button variant="subtle" asChild className="-ml-3">
					<Link to={BACK_HREF}>
						<ArrowLeftIcon />
						<span>Back to applications</span>
					</Link>
				</Button>
				{canDeleteApp && (
					<Button
						type="button"
						variant="destructive"
						disabled={isMutating}
						onClick={() => setDeleteDialogOpen(true)}
					>
						<span>Delete</span>
					</Button>
				)}
			</div>

			<div className="flex flex-col gap-6 pt-6">
				<div className="flex items-center gap-4 min-w-0">
					<Avatar
						variant="icon"
						size="lg"
						src={iconOverride ?? app.icon}
						fallback={app.name}
					/>
					<SettingsHeaderTitle>
						<span className="block min-w-0 truncate">{app.name}</span>
					</SettingsHeaderTitle>
				</div>

				<p className="text-sm text-content-secondary m-0">
					Configure this application to use Coder as an OAuth2 provider.
				</p>

				{searchParams.has("created") && (
					<Alert severity="info" dismissible>
						Your OAuth2 application has been created. Generate a client secret
						below to start using your application.
					</Alert>
				)}

				<dl className="m-0 flex flex-col gap-1.5">
					<EndpointField label="Client ID" value={app.id} />
					<EndpointField
						label="Authorization URL"
						value={app.endpoints.authorization}
					/>
					<EndpointField label="Token URL" value={app.endpoints.token} />
				</dl>

				{secretsQuery.error ? (
					<ErrorAlert error={secretsQuery.error} />
				) : undefined}

				<div className="border border-solid p-6 rounded-lg flex flex-col gap-4">
					<h2 className="m-0 text-xl font-semibold">Settings</h2>
					<OAuth2AppForm
						key={app.id}
						app={app}
						onSubmit={async (req) => {
							try {
								const updated = await putAppMutation.mutateAsync({
									id: appId,
									req,
								});
								toast.success(
									`Successfully updated the OAuth2 application "${updated.name}".`,
								);
							} catch (error) {
								toast.error(
									getErrorMessage(
										error,
										`Failed to update "${req.name}" OAuth2 application.`,
									),
									{ description: getErrorDetail(error) },
								);
							}
						}}
						isUpdating={putAppMutation.isPending}
						error={putAppMutation.error}
						disabled={!canEditApp}
						onIconChange={setIconOverride}
					/>
				</div>

				{canViewAppSecrets && (
					<div className="border border-solid p-6 rounded-lg flex flex-col gap-4">
						<div className="flex flex-row gap-4 items-center justify-between">
							<h2 className="m-0 text-xl font-semibold">Client secrets</h2>
							<Button
								disabled={postSecretMutation.isPending || isMutating}
								type="button"
								onClick={() => {
									postSecretMutation.mutate(appId, {
										onSuccess: (secret) => {
											setFullNewSecret(secret);
											toast.success(
												"Successfully generated OAuth2 client secret.",
											);
										},
										onError: (error) => {
											toast.error(
												getErrorMessage(
													error,
													"Failed to generate OAuth2 client secret.",
												),
												{ description: getErrorDetail(error) },
											);
										},
									});
								}}
							>
								<Spinner loading={postSecretMutation.isPending} />
								Generate secret
							</Button>
						</div>

						<Table aria-label="OAuth2 client secrets">
							<TableHeader>
								<TableRow>
									<TableHead className="w-[80%]">Secret</TableHead>
									<TableHead className="w-[20%]">Last used</TableHead>
									<TableHead className="w-[1%]" />
								</TableRow>
							</TableHeader>
							<TableBody size="lg">
								{secretsQuery.isLoading && <TableLoader />}
								{!secretsQuery.isLoading &&
									!secretsQuery.error &&
									(!secretsQuery.data || secretsQuery.data.length === 0) && (
										<TableEmpty message="No client secrets have been generated." />
									)}
								{!secretsQuery.isLoading &&
									secretsQuery.data?.map((secret) => (
										<OAuth2SecretRow
											key={secret.id}
											secret={secret}
											isDeleting={deleteSecretMutation.isPending}
											onDelete={(secretId) => {
												deleteSecretMutation.mutate(
													{ appId, secretId },
													{
														onSuccess: () => {
															if (fullNewSecret?.id === secretId) {
																setFullNewSecret(undefined);
															}
															toast.success(
																"Successfully deleted an OAuth2 client secret.",
															);
														},
														onError: (error) => {
															toast.error(
																getErrorMessage(
																	error,
																	"Failed to delete OAuth2 client secret.",
																),
																{ description: getErrorDetail(error) },
															);
														},
													},
												);
											}}
										/>
									))}
							</TableBody>
						</Table>
					</div>
				)}
			</div>

			{fullNewSecret && (
				<ConfirmDialog
					hideCancel
					open={Boolean(fullNewSecret)}
					onConfirm={() => setFullNewSecret(undefined)}
					onClose={() => setFullNewSecret(undefined)}
					title="OAuth2 client secret"
					confirmText="OK"
					description={
						<>
							<p>
								Your new client secret is displayed below. Make sure to copy it
								now; you will not be able to see it again.
							</p>
							<CodeExample
								code={fullNewSecret.client_secret_full}
								className="min-h-auto select-all w-full mt-6"
							/>
						</>
					}
				/>
			)}

			<DeleteDialog
				key={app.name}
				isOpen={deleteDialogOpen}
				title="Delete OAuth2 application"
				entity="OAuth2 application"
				name={app.name}
				info="Deleting this OAuth2 application will immediately invalidate all active sessions and API keys associated with it. Users currently authenticated through this application will be logged out and need to re-authenticate."
				confirmLoading={deleteAppMutation.isPending}
				onCancel={() => setDeleteDialogOpen(false)}
				onConfirm={() => {
					deleteAppMutation.mutate(appId, {
						onSuccess: () => {
							toast.success(
								`You have successfully deleted the "${app.name}" OAuth2 application.`,
							);
							setDeleteDialogOpen(false);
							void navigate(BACK_HREF, { replace: true });
						},
						onError: (error) => {
							toast.error(
								getErrorMessage(
									error,
									`Failed to delete "${app.name}" OAuth2 application.`,
								),
								{ description: getErrorDetail(error) },
							);
						},
					});
				}}
			/>
		</>
	);
};

type EndpointFieldProps = {
	label: string;
	value: string;
};

const EndpointField: FC<EndpointFieldProps> = ({ label, value }) => {
	return (
		<div className="flex items-center gap-2">
			<dt className="text-sm">{label}</dt>
			<dd className="m-0">
				<div className="flex items-center gap-0.5">
					<code className="w-fit rounded-md bg-surface-secondary px-2 py-0.5 font-mono text-xs text-content-secondary">
						{value}
					</code>
					<CopyButton
						text={value}
						label={`Copy ${label}`}
						size="icon"
						variant="subtle"
					/>
				</div>
			</dd>
		</div>
	);
};

type OAuth2SecretRowProps = {
	secret: TypesGen.OAuth2ProviderAppSecret;
	onDelete: (id: string) => void;
	isDeleting: boolean;
};

const OAuth2SecretRow: FC<OAuth2SecretRowProps> = ({
	secret,
	onDelete,
	isDeleting,
}) => {
	const [showDelete, setShowDelete] = useState(false);

	return (
		<TableRow data-testid={`secret-${secret.id}`}>
			<TableCell>*****{secret.client_secret_truncated}</TableCell>
			<TableCell data-pixel="ignore">
				{secret.last_used_at ? createDayString(secret.last_used_at) : "Never"}
			</TableCell>
			<TableCell>
				<ConfirmDialog
					type="delete"
					hideCancel={false}
					open={showDelete}
					onConfirm={() => {
						onDelete(secret.id);
						setShowDelete(false);
					}}
					onClose={() => setShowDelete(false)}
					title="Delete OAuth2 client secret"
					confirmLoading={isDeleting}
					confirmText="Delete"
					description={
						<>
							Deleting <strong>*****{secret.client_secret_truncated}</strong> is
							irreversible and will revoke all the tokens generated by it. Are
							you sure you want to proceed?
						</>
					}
				/>
				<Button variant="destructive" onClick={() => setShowDelete(true)}>
					Delete secret
				</Button>
			</TableCell>
		</TableRow>
	);
};
