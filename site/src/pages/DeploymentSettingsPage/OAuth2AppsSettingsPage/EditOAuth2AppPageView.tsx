import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import CopyIcon from "@mui/icons-material/FileCopyOutlined";
import LoadingButton from "@mui/lab/LoadingButton";
import Divider from "@mui/material/Divider";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import { ChevronLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import { OAuth2AppForm } from "./OAuth2AppForm";

export type MutatingResource = {
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
	const theme = useTheme();
	const [searchParams] = useSearchParams();
	const [showDelete, setShowDelete] = useState<boolean>(false);

	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="Edit OAuth2 application"
					description="Configure an application to use Coder as an OAuth2 provider."
				/>
				<Button variant="outline" asChild>
					<RouterLink to="/deployment/oauth2-provider/apps">
						<ChevronLeftIcon />
						All OAuth2 Applications
					</RouterLink>
				</Button>
			</Stack>

			{fullNewSecret && (
				<ConfirmDialog
					hideCancel
					open={Boolean(fullNewSecret)}
					onConfirm={ackFullNewSecret}
					onClose={ackFullNewSecret}
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
								css={{
									minHeight: "auto",
									userSelect: "all",
									width: "100%",
									marginTop: 24,
								}}
							/>
						</>
					}
				/>
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
							onConfirm={() => deleteApp(app.name)}
							onCancel={() => setShowDelete(false)}
						/>

						<dl css={styles.dataList}>
							<dt>Client ID</dt>
							<dd>
								<CopyableValue value={app.id}>
									{app.id} <CopyIcon css={{ width: 16, height: 16 }} />
								</CopyableValue>
							</dd>
							<dt>Authorization URL</dt>
							<dd>
								<CopyableValue value={app.endpoints.authorization}>
									{app.endpoints.authorization}{" "}
									<CopyIcon css={{ width: 16, height: 16 }} />
								</CopyableValue>
							</dd>
							<dt>Token URL</dt>
							<dd>
								<CopyableValue value={app.endpoints.token}>
									{app.endpoints.token}{" "}
									<CopyIcon css={{ width: 16, height: 16 }} />
								</CopyableValue>
							</dd>
						</dl>

						<Divider css={{ borderColor: theme.palette.divider }} />

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

						<Divider css={{ borderColor: theme.palette.divider }} />

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
				<h2>Client secrets</h2>
				<LoadingButton
					loading={mutatingResource.createSecret}
					type="submit"
					variant="contained"
					onClick={generateAppSecret}
				>
					Generate secret
				</LoadingButton>
			</Stack>

			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="80%">Secret</TableCell>
							<TableCell width="20%">Last Used</TableCell>
							<TableCell width="1%" />
						</TableRow>
					</TableHead>
					<TableBody>
						{isLoadingSecrets && <TableLoader />}
						{!isLoadingSecrets && (!secrets || secrets.length === 0) && (
							<TableRow>
								<TableCell colSpan={999}>
									<div css={{ textAlign: "center" }}>
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
			</TableContainer>
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

const styles = {
	dataList: {
		display: "grid",
		gridTemplateColumns: "max-content auto",
		"& > dt": {
			fontWeight: "bold",
		},
		"& > dd": {
			marginLeft: 10,
		},
	},
} satisfies Record<string, Interpolation<Theme>>;
