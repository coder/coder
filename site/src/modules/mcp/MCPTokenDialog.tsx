import { type FormikErrors, useFormik } from "formik";
import { useId } from "react";
import { isApiValidationError, mapApiErrorToFieldErrors } from "#/api/errors";
import type {
	MCPServerCredentialKind,
	MCPServerGitHubApp,
	MCPServerGitHubAppCredentialRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { getFormHelpers, passwordManagerIgnoreProps } from "#/utils/formUtils";

type MCPTokenDialogProps = {
	/** The name of the MCP server the credential is for. */
	readonly serverName: string;
	/** Says whose chats use the credential, for the dialog's description. */
	readonly audience: string;
	/** True when a credential is stored, so saving replaces it. */
	readonly hasCredential: boolean;
	/** Where to find the token, such as "personal access token". */
	readonly help?: { readonly label: string; readonly url: string };
	readonly isSaving: boolean;
	readonly isRemoving: boolean;
	/** Why the last save or removal failed. */
	readonly error: unknown;
	readonly onSave: (token: string) => void;
	/**
	 * Offers a GitHub App as the credential instead of a token. Pass it only
	 * for a GitHub server of an agent: personal tokens cannot use an App.
	 */
	readonly githubApp?: {
		/** The stored App, when the server uses one. */
		readonly current?: MCPServerGitHubApp;
		readonly onSave: (app: MCPServerGitHubAppCredentialRequest) => void;
	};
	/** Omit when there is nothing to remove. */
	readonly onRemove?: () => void;
	readonly onClose: () => void;
};

type CredentialFormValues = {
	kind: MCPServerCredentialKind;
	token: string;
	appID: string;
	installationID: string;
	privateKey: string;
	repositories: string;
	useForGit: boolean;
};

const privateKeyField = "github_app.private_key";
const repositoriesField = "github_app.repositories";

const parseID = (value: string) => Number.parseInt(value, 10);

const parseRepositories = (value: string) =>
	value
		.split(/[,\n]/)
		.map((repository) => repository.trim())
		.filter(Boolean);

/**
 * Pastes, replaces, or removes the credential of an MCP server that asks each
 * user for their own. The credential is write-only: it is never shown again.
 */
export const MCPTokenDialog: React.FC<MCPTokenDialogProps> = ({
	serverName,
	audience,
	hasCredential,
	help,
	isSaving,
	isRemoving,
	error,
	onSave,
	githubApp,
	onRemove,
	onClose,
}) => {
	const kindGroupId = useId();
	const isBusy = isSaving || isRemoving;
	const storedApp = githubApp?.current;
	const credentialNoun = githubApp ? "credential" : "token";

	const validateValues = (values: CredentialFormValues) => {
		const errors: FormikErrors<CredentialFormValues> = {};
		if (values.kind === "token") {
			if (values.token.trim() === "") {
				errors.token = "Enter a token.";
			}
			return errors;
		}
		const appID = parseID(values.appID);
		if (!(appID > 0)) {
			errors.appID = "Enter the numeric App ID.";
		}
		if (!(parseID(values.installationID) > 0)) {
			errors.installationID = "Enter the numeric installation ID.";
		}
		// The stored key only carries over while the App stays the same.
		const keepsStoredKey = storedApp?.app_id === appID;
		if (values.privateKey.trim() === "" && !keepsStoredKey) {
			errors.privateKey = "Paste the App's private key.";
		}
		return errors;
	};

	const form = useFormik<CredentialFormValues>({
		initialValues: {
			kind: storedApp ? "github_app" : "token",
			token: "",
			appID: storedApp?.app_id.toString() ?? "",
			installationID: storedApp?.installation_id.toString() ?? "",
			privateKey: "",
			repositories: storedApp?.repositories.join(", ") ?? "",
			useForGit: storedApp?.use_for_git ?? false,
		},
		validateOnMount: true,
		validate: validateValues,
		onSubmit: (values) => {
			if (values.kind === "github_app" && githubApp) {
				githubApp.onSave({
					app_id: parseID(values.appID),
					installation_id: parseID(values.installationID),
					private_key: values.privateKey.trim() || undefined,
					repositories: parseRepositories(values.repositories),
					use_for_git: values.useForGit,
				});
				return;
			}
			onSave(values.token.trim());
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const canSave = Object.keys(validateValues(form.values)).length === 0;
	const isGitHubApp =
		githubApp !== undefined && form.values.kind === "github_app";
	const fieldErrors = isApiValidationError(error)
		? mapApiErrorToFieldErrors(error.response.data)
		: {};
	const hasFieldError =
		fieldErrors[privateKeyField] !== undefined ||
		fieldErrors[repositoriesField] !== undefined;

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open && !isBusy) {
					onClose();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{hasCredential ? "Replace" : "Add"} {credentialNoun} for{" "}
						{serverName}
					</DialogTitle>
					<DialogDescription>
						{audience} use this {credentialNoun} to call {serverName}. It is
						stored encrypted and cannot be read back.
					</DialogDescription>
				</DialogHeader>
				<form
					className="flex flex-col gap-4"
					onSubmit={(event) => {
						if (isBusy || !canSave) {
							event.preventDefault();
							return;
						}
						form.handleSubmit(event);
					}}
				>
					{githubApp && (
						<RadioGroup
							value={form.values.kind}
							onValueChange={(value) => void form.setFieldValue("kind", value)}
							aria-label="Credential"
							className="gap-2"
						>
							<RadioOption
								id={`${kindGroupId}-token`}
								value="token"
								label="Paste a token"
							/>
							<RadioOption
								id={`${kindGroupId}-github-app`}
								value="github_app"
								label="Use a GitHub App"
							/>
						</RadioGroup>
					)}
					{isGitHubApp ? (
						<GitHubAppCredentialFields
							getFieldHelpers={getFieldHelpers}
							form={form}
							hasStoredApp={storedApp !== undefined}
							disabled={isBusy}
						/>
					) : (
						<TokenField
							getFieldHelpers={getFieldHelpers}
							help={help}
							disabled={isBusy}
						/>
					)}
					{Boolean(error) && !hasFieldError && (
						<ErrorAlert error={error} showDebugDetail={false} />
					)}
					<DialogFooter>
						{onRemove && hasCredential && (
							<Button
								type="button"
								variant="outline"
								className="sm:mr-auto"
								disabled={isBusy}
								onClick={onRemove}
							>
								<Spinner loading={isRemoving} />
								Remove {credentialNoun}
							</Button>
						)}
						<Button
							type="button"
							variant="outline"
							disabled={isBusy}
							onClick={onClose}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={!canSave || isBusy}>
							<Spinner loading={isSaving} />
							{hasCredential ? "Replace" : "Save"} {credentialNoun}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

type FieldHelpersGetter = ReturnType<
	typeof getFormHelpers<CredentialFormValues>
>;

type TokenFieldProps = {
	readonly getFieldHelpers: FieldHelpersGetter;
	readonly help?: MCPTokenDialogProps["help"];
	readonly disabled: boolean;
};

const TokenField: React.FC<TokenFieldProps> = ({
	getFieldHelpers,
	help,
	disabled,
}) => (
	<FormField
		field={getFieldHelpers("token")}
		label="Token"
		type="password"
		autoFocus
		ignorePasswordManagers
		disabled={disabled}
		description={
			help && (
				<>
					Use a {help.label}.{" "}
					<Link href={help.url} target="_blank" rel="noreferrer" size="sm">
						Where to find it
					</Link>
				</>
			)
		}
	/>
);

type GitHubAppCredentialFieldsProps = {
	readonly getFieldHelpers: FieldHelpersGetter;
	readonly form: ReturnType<typeof useFormik<CredentialFormValues>>;
	readonly hasStoredApp: boolean;
	readonly disabled: boolean;
};

const GitHubAppCredentialFields: React.FC<GitHubAppCredentialFieldsProps> = ({
	getFieldHelpers,
	form,
	hasStoredApp,
	disabled,
}) => {
	const useForGitId = useId();
	const useForGitWarningId = useId();
	return (
		<div className="flex flex-col gap-4">
			<p className="m-0 text-xs text-content-secondary">
				Coder mints short-lived tokens for the App's installation. After you
				change the App's permissions on GitHub, save here again to use them
				right away; otherwise they apply within 10 minutes.
			</p>
			<div className="grid gap-4 sm:grid-cols-2">
				<FormField
					field={getFieldHelpers("appID")}
					label="App ID"
					inputMode="numeric"
					disabled={disabled}
				/>
				<FormField
					field={getFieldHelpers("installationID")}
					label="Installation ID"
					inputMode="numeric"
					disabled={disabled}
				/>
			</div>
			<FormField
				field={getFieldHelpers("privateKey", {
					backendFieldName: privateKeyField,
				})}
				label="Private key"
				description={
					hasStoredApp
						? "Leave empty to keep the stored key. That works only while the App ID is unchanged."
						: "The App's PEM private key. It is stored encrypted and cannot be read back."
				}
				control={(props) => (
					<Textarea
						{...props}
						{...form.getFieldProps("privateKey")}
						rows={5}
						className="font-mono"
						placeholder={
							hasStoredApp
								? "Stored. Paste a new key to replace it."
								: "-----BEGIN RSA PRIVATE KEY-----"
						}
						{...passwordManagerIgnoreProps}
						disabled={disabled}
					/>
				)}
			/>
			<FormField
				field={getFieldHelpers("repositories", {
					backendFieldName: repositoriesField,
				})}
				label="Repositories (optional)"
				description="Separate with commas or new lines. Leave empty for every repository the installation can access."
				control={(props) => (
					<Textarea
						{...props}
						{...form.getFieldProps("repositories")}
						rows={2}
						placeholder="owner/repo, other-repo"
						disabled={disabled}
					/>
				)}
			/>
			<div className="flex items-start gap-2">
				<Checkbox
					id={useForGitId}
					checked={form.values.useForGit}
					onCheckedChange={(checked) =>
						void form.setFieldValue("useForGit", checked === true)
					}
					disabled={disabled}
					aria-describedby={useForGitWarningId}
					className="mt-0.5"
				/>
				<div className="flex flex-col gap-1">
					<Label htmlFor={useForGitId} className="cursor-pointer font-normal">
						Also use this identity for git in the agent's workspaces
					</Label>
					<p
						id={useForGitWarningId}
						className="m-0 text-xs text-content-secondary"
					>
						The App token is then available to commands in the agent's
						workspaces, so anyone who can use the agent could read it. List
						repositories above to limit what git can reach.
					</p>
				</div>
			</div>
		</div>
	);
};

type RadioOptionProps = {
	readonly id: string;
	readonly value: MCPServerCredentialKind;
	readonly label: string;
};

const RadioOption: React.FC<RadioOptionProps> = ({ id, value, label }) => (
	<div className="flex items-center gap-2">
		<RadioGroupItem id={id} value={value} />
		<Label htmlFor={id} className="cursor-pointer font-normal">
			{label}
		</Label>
	</div>
);
