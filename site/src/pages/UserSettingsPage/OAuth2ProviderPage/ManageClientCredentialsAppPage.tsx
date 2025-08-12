import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import DialogActions from "@mui/material/DialogActions";
import Link from "@mui/material/Link";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { getErrorMessage } from "api/errors";
import {
	deleteApp,
	deleteAppSecret,
	getApp,
	getAppSecrets,
	postAppSecret,
	putApp,
} from "api/queries/oauth2";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button as CustomButton } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { ClientCredentialsAppForm } from "components/OAuth2/ClientCredentialsAppForm";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import dayjs from "dayjs";
import { ChevronLeftIcon, CopyIcon, PlusIcon, TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import { Section } from "../Section";

const ManageClientCredentialsAppPage: FC = () => {
	const { appId } = useParams<{ appId: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	if (!appId) {
		navigate("/settings/oauth2-provider");
		return null;
	}

	const [editMode, setEditMode] = useState(false);
	const [secretToDelete, setSecretToDelete] = useState<string>();
	const [newSecretFull, setNewSecretFull] =
		useState<TypesGen.OAuth2ProviderAppSecretFull | null>(null);
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);
	const [showCodeExample, setShowCodeExample] = useState(false);

	const appQuery = useQuery(getApp(appId));
	const secretsQuery = useQuery(getAppSecrets(appId));
	const updateAppMutation = useMutation(putApp(queryClient));
	const createSecretMutation = useMutation(postAppSecret(queryClient));
	const deleteSecretMutation = useMutation(deleteAppSecret(queryClient));
	const deleteAppMutation = useMutation(deleteApp(queryClient));

	const app = appQuery.data;
	const secrets = secretsQuery.data || [];
	const secretToDeleteData = secrets.find((s) => s.id === secretToDelete);

	const handleUpdateApp = async (data: {
		name: string;
		icon: string;
		grant_types: TypesGen.OAuth2ProviderGrantType[];
		redirect_uris: string[];
	}) => {
		if (!app) return;

		try {
			await updateAppMutation.mutateAsync({
				id: app.id,
				req: {
					name: data.name,
					icon: data.icon,
					grant_types: data.grant_types,
					redirect_uris: data.redirect_uris,
				},
			});
			displaySuccess("Application updated successfully!");
			setEditMode(false);
		} catch (error) {
			displayError(getErrorMessage(error, "Failed to update application."));
		}
	};

	const handleCreateSecret = async () => {
		try {
			const newSecret = await createSecretMutation.mutateAsync(appId);
			setNewSecretFull(newSecret);
			displaySuccess("Client secret created successfully!");
		} catch (error) {
			displayError(getErrorMessage(error, "Failed to create client secret."));
		}
	};

	const handleDeleteApp = async () => {
		try {
			await deleteAppMutation.mutateAsync(appId);
			displaySuccess("Application deleted successfully!");
			navigate("/settings/oauth2-provider");
		} catch (error) {
			displayError(getErrorMessage(error, "Failed to delete application."));
		}
	};

	if (appQuery.isLoading) {
		return (
			<Section title="Loading..." layout="fluid">
				<Spinner />
			</Section>
		);
	}

	if (appQuery.error || !app) {
		return (
			<Section title="Application Not Found" layout="fluid">
				<ErrorAlert
					error={appQuery.error || new Error("Application not found")}
				/>
			</Section>
		);
	}

	return (
		<Stack spacing={4}>
			{editMode ? (
				<ClientCredentialsAppForm
					app={app}
					onSubmit={handleUpdateApp}
					error={updateAppMutation.error}
					isUpdating={updateAppMutation.isPending}
				/>
			) : (
				<>
					<Stack
						alignItems="baseline"
						direction="row"
						justifyContent="space-between"
					>
						<Section
							title={app.name}
							description="OAuth2 client credentials application"
							layout="fluid"
						/>

						<Stack direction="row" spacing={2}>
							<CustomButton onClick={() => setEditMode(true)}>
								Edit
							</CustomButton>
							<CustomButton
								variant="destructive"
								onClick={() => setShowDeleteDialog(true)}
							>
								Delete
							</CustomButton>
							<CustomButton variant="outline" asChild>
								<RouterLink to="/settings/oauth2-provider">
									<ChevronLeftIcon />
									All OAuth2 Applications
								</RouterLink>
							</CustomButton>
						</Stack>
					</Stack>

					{/* Application details using admin pattern */}
					<dl className="grid grid-cols-[max-content_auto] [&>dt]:font-bold [&>dd]:ml-2.5">
						<dt>Client ID</dt>
						<dd>
							<CopyableValue value={app.id}>
								{app.id} <CopyIcon className="size-icon-xs" />
							</CopyableValue>
						</dd>
						<dt>Token URL</dt>
						<dd>
							<CopyableValue value={app.endpoints.token}>
								{app.endpoints.token} <CopyIcon className="size-icon-xs" />
							</CopyableValue>
						</dd>
						<dt>Grant Types</dt>
						<dd>{app.grant_types?.join(", ") || "None"}</dd>
						<dt>Created</dt>
						<dd>{dayjs(app.created_at).format("MMM D, YYYY [at] h:mm A")}</dd>
					</dl>
				</>
			)}

			{!editMode && (
				<Section title="Client Secrets" layout="fluid">
					<Stack spacing={3}>
						<div className="flex items-center justify-between">
							<p className="text-sm text-content-secondary">
								Client secrets are used to authenticate your application with
								the Coder API.
							</p>
							<Button
								startIcon={<PlusIcon className="size-icon-sm" />}
								onClick={handleCreateSecret}
								disabled={createSecretMutation.isPending}
								variant="contained"
							>
								<Spinner loading={createSecretMutation.isPending} />
								Create Secret
							</Button>
						</div>

						{secretsQuery.error && <ErrorAlert error={secretsQuery.error} />}

						<TableContainer>
							<Table>
								<TableHead>
									<TableRow>
										<TableCell>Secret</TableCell>
										<TableCell>Last Used</TableCell>
										<TableCell width="1%">Actions</TableCell>
									</TableRow>
								</TableHead>
								<TableBody>
									{secretsQuery.isLoading && <TableLoader />}
									{secrets.map((secret) => (
										<TableRow key={secret.id}>
											<TableCell>
												<code className="rounded bg-surface-secondary px-2 py-1 text-sm">
													*****{secret.client_secret_truncated}
												</code>
											</TableCell>
											<TableCell>
												{secret.last_used_at
													? dayjs(secret.last_used_at).format("MMM D, YYYY")
													: "Never"}
											</TableCell>
											<TableCell>
												<Button
													size="small"
													color="error"
													onClick={() => setSecretToDelete(secret.id)}
												>
													<TrashIcon className="size-icon-xs" />
												</Button>
											</TableCell>
										</TableRow>
									))}
									{secrets.length === 0 && !secretsQuery.isLoading && (
										<TableRow>
											<TableCell colSpan={4}>
												<div className="text-center py-8 px-4">
													<p className="text-content-secondary">
														No client secrets created yet.
													</p>
													<p className="text-sm text-content-secondary mt-1">
														Create a secret to start using this application.
													</p>
												</div>
											</TableCell>
										</TableRow>
									)}
								</TableBody>
							</Table>
						</TableContainer>
					</Stack>
				</Section>
			)}

			{secretToDeleteData && (
				<ConfirmDialog
					type="delete"
					hideCancel={false}
					open={Boolean(secretToDeleteData)}
					onConfirm={async () => {
						try {
							await deleteSecretMutation.mutateAsync({
								appId,
								secretId: secretToDeleteData.id,
							});
							displaySuccess("Client secret deleted successfully!");
							setSecretToDelete(undefined);
						} catch (error) {
							displayError(
								getErrorMessage(error, "Failed to delete client secret."),
							);
						}
					}}
					onClose={() => setSecretToDelete(undefined)}
					title="Delete client secret"
					confirmLoading={deleteSecretMutation.isPending}
					confirmText="Delete"
					description={
						<>
							Deleting{" "}
							<strong>*****{secretToDeleteData.client_secret_truncated}</strong>{" "}
							is irreversible and will revoke all the tokens generated by it.
							Are you sure you want to proceed?
						</>
					}
				/>
			)}

			{/* New Secret Dialog - Show full secret once */}
			{newSecretFull && app && (
				<Dialog
					PaperProps={{
						className: `w-full transition-all duration-500 ease-in-out overflow-hidden ${showCodeExample ? "max-w-[800px]" : "max-w-[440px]"}`,
					}}
					open={Boolean(newSecretFull)}
					onClose={() => {
						setNewSecretFull(null);
						setShowCodeExample(false);
					}}
					data-testid="dialog"
				>
					<div className="text-content-secondary px-10 pt-10 pb-5">
						<h3 className="m-0 mb-4 text-content-primary font-normal text-xl">
							Client secret created
						</h3>
						<div className="text-content-secondary leading-relaxed text-base [&_strong]:text-content-primary [&_p:not(.MuiFormHelperText-root)]:m-0 [&>p]:my-2">
							<p>
								Your new client secret is displayed below. Make sure to copy it
								now; you will not be able to see it again.
							</p>
							<CodeExample
								code={newSecretFull.client_secret_full}
								secret={false}
								className="min-h-auto select-all w-full mt-6"
							/>
							{app.grant_types?.includes("client_credentials") && (
								<div className="mt-6">
									<Link
										onClick={() => setShowCodeExample(!showCodeExample)}
										className="cursor-pointer text-content-secondary flex items-center text-sm"
									>
										Code Example
										<DropdownArrow margin={false} close={showCodeExample} />
									</Link>
									<Collapse in={showCodeExample} timeout={300}>
										<div className="mt-4">
											<p className="mb-4 text-sm text-content-secondary">
												Use this curl command to exchange your client
												credentials for an access token:
											</p>
											<CodeExample
												code={`curl -X POST "${app.endpoints.token}" \\
  -H "Content-Type: application/x-www-form-urlencoded" \\
  -d "grant_type=client_credentials" \\
  -d "client_id=${app.id}" \\
  -d "client_secret=${newSecretFull.client_secret_full}"`}
												secret={false}
												className="min-h-auto select-all w-full whitespace-pre overflow-x-auto"
											/>
										</div>
									</Collapse>
								</div>
							)}
						</div>
					</div>

					<DialogActions className="px-10 pb-10">
						<DialogActionButtons
							confirmLoading={false}
							confirmText="I've Saved the Secret"
							disabled={false}
							onCancel={undefined}
							onConfirm={() => {
								setNewSecretFull(null);
								setShowCodeExample(false);
							}}
						/>
					</DialogActions>
				</Dialog>
			)}

			{/* Delete Application Dialog */}
			{showDeleteDialog && (
				<DeleteDialog
					title="Delete OAuth2 Application"
					verb="Deleting"
					info={`This will permanently delete the application "${app.name}" and all its client secrets. Any applications using these credentials will stop working.`}
					label="Application name"
					isOpen
					confirmLoading={deleteAppMutation.isPending}
					name={app.name}
					entity="application"
					onCancel={() => setShowDeleteDialog(false)}
					onConfirm={handleDeleteApp}
				/>
			)}
		</Stack>
	);
};

export default ManageClientCredentialsAppPage;
