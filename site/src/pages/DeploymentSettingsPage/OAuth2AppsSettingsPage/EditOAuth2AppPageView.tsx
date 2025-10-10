import Collapse from "@mui/material/Collapse";
import DialogActions from "@mui/material/DialogActions";
import Divider from "@mui/material/Divider";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
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
import dayjs from "dayjs";
import { ChevronLeftIcon, CopyIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink, useSearchParams } from "react-router";
import { createDayString } from "utils/createDayString";
import { OAuth2AppForm } from "./OAuth2AppForm";

type MutatingResource = {
	updateApp: boolean;
	createSecret: boolean;
	deleteApp: boolean;
	deleteSecret: boolean;
};

type EditOAuth2AppProps = {
	app?: TypesGen.OAuth2ProviderApp;
	isLoadingApp: boolean;
	isLoadingSecrets: boolean;
	// mutatingResource indicates which resources, if any, are currently being
	// mutated.
	mutatingResource: MutatingResource;
	updateApp: (req: TypesGen.PutOAuth2ProviderAppRequest) => void;
	deleteApp: (name: string) => void;
	generateAppSecret: () => void;
	deleteAppSecret: (id: string) => void;
	secrets?: readonly TypesGen.OAuth2ProviderAppSecret[];
	fullNewSecret?: TypesGen.OAuth2ProviderAppSecretFull;
	ackFullNewSecret: () => void;
	error?: unknown;
};

export const EditOAuth2AppPageView: FC<EditOAuth2AppProps> = ({
	app,
	isLoadingApp,
	isLoadingSecrets,
	mutatingResource,
	updateApp,
	deleteApp,
	generateAppSecret,
	deleteAppSecret,
	secrets,
	fullNewSecret,
	ackFullNewSecret,
	error,
}) => {
	const [searchParams] = useSearchParams();
	const [showDelete, setShowDelete] = useState<boolean>(false);
	const [showCodeExample, setShowCodeExample] = useState(false);

	const owner =
		app?.user_id && app.user_id !== "00000000-0000-0000-0000-000000000000"
			? { id: app.user_id, username: app.username, email: app.email }
			: null;

	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader>
					<SettingsHeaderTitle>Edit OAuth2 application</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Configure an application to use Coder as an OAuth2 provider.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Button variant="outline" asChild>
					<RouterLink to="/deployment/oauth2-provider/apps">
						<ChevronLeftIcon />
						All OAuth2 Applications
					</RouterLink>
				</Button>
			</Stack>

			{fullNewSecret && app && (
				<Dialog
					open={Boolean(fullNewSecret)}
					onClose={() => {
						ackFullNewSecret();
						setShowCodeExample(false);
					}}
					data-testid="dialog"
				>
					<div className="p-10 pb-5 text-content-secondary">
						<h3 className="m-0 mb-4 text-content-primary font-normal text-xl">
							OAuth2 client secret
						</h3>
						<div className="text-content-secondary leading-relaxed text-base">
							<p className="my-2">
								Your new client secret is displayed below. Make sure to copy it
								now; you will not be able to see it again.
							</p>
							<CodeExample
								code={fullNewSecret.client_secret_full}
								secret={false}
								className="mt-6 min-h-auto select-all w-full"
							/>
							{app.grant_types?.includes("client_credentials") && (
								<div className="mt-6">
									<Button
										variant="subtle"
										onClick={() => setShowCodeExample(!showCodeExample)}
										className="flex items-center p-0 text-content-secondary text-sm"
									>
										Code Example
										<DropdownArrow margin={false} close={showCodeExample} />
									</Button>
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
  -d "client_secret=${fullNewSecret.client_secret_full}"`}
												secret={false}
												className="min-h-auto select-all w-full whitespace-pre overflow-x-auto"
											/>
										</div>
									</Collapse>
								</div>
							)}
						</div>
					</div>

					<DialogActions className="p-10 pt-0">
						<DialogActionButtons
							confirmLoading={false}
							confirmText="OK"
							disabled={false}
							onCancel={undefined}
							onConfirm={() => {
								ackFullNewSecret();
								setShowCodeExample(false);
							}}
						/>
					</DialogActions>
				</Dialog>
			)}

			<Stack>
				{searchParams.has("created") && (
					<Alert severity="info" dismissible>
						Your OAuth2 application has been created. Generate a client secret
						below to start using your application.
					</Alert>
				)}

				{error ? <ErrorAlert error={error} /> : undefined}

				{isLoadingApp && <Loader />}

				{!isLoadingApp && app && (
					<>
						<DeleteDialog
							isOpen={showDelete}
							confirmLoading={mutatingResource.deleteApp}
							name={app.name}
							entity="OAuth2 application"
							info="Deleting this OAuth2 application will immediately invalidate all active sessions and API keys associated with it. Users currently authenticated through this application will be logged out and need to re-authenticate."
							onConfirm={() => deleteApp(app.name)}
							onCancel={() => setShowDelete(false)}
						/>

						<dl className="grid grid-cols-[max-content_auto] gap-x-2.5 gap-y-2">
							<dt className="font-bold">Client ID</dt>
							<dd>
								<CopyableValue value={app.id}>
									{app.id} <CopyIcon className="size-icon-xs" />
								</CopyableValue>
							</dd>
							<dt className="font-bold">Authorization URL</dt>
							<dd>
								<CopyableValue value={app.endpoints.authorization}>
									{app.endpoints.authorization}{" "}
									<CopyIcon className="size-icon-xs" />
								</CopyableValue>
							</dd>
							<dt className="font-bold">Token URL</dt>
							<dd>
								<CopyableValue value={app.endpoints.token}>
									{app.endpoints.token} <CopyIcon className="size-icon-xs" />
								</CopyableValue>
							</dd>
							<dt className="font-bold">Grant Types</dt>
							<dd>{app.grant_types?.join(", ") || "None"}</dd>
							<dt className="font-bold">Created</dt>
							<dd>{dayjs(app.created_at).format("MMM D, YYYY [at] h:mm A")}</dd>
							{owner && (
								<>
									<dt className="font-bold">Owner</dt>
									<dd>{owner.username || owner.email || owner.id}</dd>
								</>
							)}
						</dl>

						<Divider className="border-border-default" />

						<OAuth2AppForm
							app={app}
							onSubmit={updateApp}
							isUpdating={mutatingResource.updateApp}
							error={error}
							actions={
								<Button
									variant="destructive"
									onClick={() => setShowDelete(true)}
								>
									Delete&hellip;
								</Button>
							}
						/>

						<Divider className="border-border-default" />

						<OAuth2AppSecretsTable
							secrets={secrets}
							generateAppSecret={generateAppSecret}
							deleteAppSecret={deleteAppSecret}
							isLoadingSecrets={isLoadingSecrets}
							mutatingResource={mutatingResource}
						/>
					</>
				)}
			</Stack>
		</>
	);
};

type OAuth2AppSecretsTableProps = {
	secrets?: readonly TypesGen.OAuth2ProviderAppSecret[];
	generateAppSecret: () => void;
	isLoadingSecrets: boolean;
	mutatingResource: MutatingResource;
	deleteAppSecret: (id: string) => void;
};

const OAuth2AppSecretsTable: FC<OAuth2AppSecretsTableProps> = ({
	secrets,
	generateAppSecret,
	isLoadingSecrets,
	mutatingResource,
	deleteAppSecret,
}) => {
	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<h2>Client Secrets</h2>
				<Button
					disabled={mutatingResource.createSecret}
					type="submit"
					onClick={generateAppSecret}
				>
					<Spinner loading={mutatingResource.createSecret} />
					Create Secret
				</Button>
			</Stack>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[80%]">Secret</TableHead>
						<TableHead className="w-[20%]">Last Used</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoadingSecrets && <TableLoader />}
					{!isLoadingSecrets && (!secrets || secrets.length === 0) && (
						<TableRow>
							<TableCell colSpan={999}>
								<div className="text-center">
									No client secrets have been generated.
								</div>
							</TableCell>
						</TableRow>
					)}
					{!isLoadingSecrets &&
						secrets?.map((secret) => (
							<OAuth2SecretRow
								key={secret.id}
								secret={secret}
								mutatingResource={mutatingResource}
								deleteAppSecret={deleteAppSecret}
							/>
						))}
				</TableBody>
			</Table>
		</>
	);
};

type OAuth2SecretRowProps = {
	secret: TypesGen.OAuth2ProviderAppSecret;
	deleteAppSecret: (id: string) => void;
	mutatingResource: MutatingResource;
};

const OAuth2SecretRow: FC<OAuth2SecretRowProps> = ({
	secret,
	deleteAppSecret,
	mutatingResource,
}) => {
	const [showDelete, setShowDelete] = useState<boolean>(false);

	return (
		<TableRow key={secret.id} data-testid={`secret-${secret.id}`}>
			<TableCell>*****{secret.client_secret_truncated}</TableCell>
			<TableCell data-chromatic="ignore">
				{secret.last_used_at ? createDayString(secret.last_used_at) : "never"}
			</TableCell>
			<TableCell>
				<ConfirmDialog
					type="delete"
					hideCancel={false}
					open={showDelete}
					onConfirm={() => deleteAppSecret(secret.id)}
					onClose={() => setShowDelete(false)}
					title="Delete OAuth2 client secret"
					confirmLoading={mutatingResource.deleteSecret}
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
					Delete&hellip;
				</Button>
			</TableCell>
		</TableRow>
	);
};
