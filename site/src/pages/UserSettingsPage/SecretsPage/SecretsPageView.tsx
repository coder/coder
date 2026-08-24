import { PlusIcon, SquareArrowOutUpRightIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type {
	CreateUserSecretRequest,
	ImportUserSecretsRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { docs } from "#/utils/docs";
import { SecretDialog } from "./SecretDialog";
import { SecretsTable } from "./SecretsTable";

type SecretsPageViewProps = {
	secrets?: readonly UserSecret[];
	// Undefined when the deployment does not report the setting, which the page
	// treats as file paths being allowed.
	filePathEnabled?: boolean;
	isLoading: boolean;
	hasLoaded: boolean;
	isCreating: boolean;
	isUpdating: boolean;
	isDeleting: boolean;
	getSecretsError?: unknown;
	onCreateSecret: (
		request: CreateUserSecretRequest,
	) => Promise<UserSecret> | UserSecret;
	onUpdateSecret: (
		name: string,
		request: UpdateUserSecretRequest,
	) => Promise<UserSecret> | UserSecret;
	onImportSecrets: (request: ImportUserSecretsRequest) => Promise<UserSecret[]>;
	onDeleteSecret: (secret: UserSecret) => Promise<void> | void;
	onToggleSecretEnabled: (
		secret: UserSecret,
		enabled: boolean,
	) => Promise<void> | void;
};

type SecretDialogState =
	| { mode: "add"; open: boolean }
	| { mode: "edit"; open: boolean; secret: UserSecret };

export const SecretsPageView: FC<SecretsPageViewProps> = ({
	secrets = [],
	filePathEnabled = true,
	isLoading,
	hasLoaded,
	isCreating,
	isUpdating,
	isDeleting,
	getSecretsError,
	onCreateSecret,
	onUpdateSecret,
	onImportSecrets,
	onDeleteSecret,
	onToggleSecretEnabled,
}) => {
	const [dialogState, setDialogState] = useState<SecretDialogState>({
		mode: "add",
		open: false,
	});
	const secretDialogReturnFocusElement = useRef<HTMLElement | null>(null);
	const dialogSecret =
		dialogState.mode === "edit" ? dialogState.secret : undefined;
	const hasLoadedSecrets = hasLoaded && !getSecretsError;

	const openAddSecret = (returnFocusElement?: HTMLElement | null) => {
		secretDialogReturnFocusElement.current = returnFocusElement ?? null;
		setDialogState({ mode: "add", open: true });
	};
	const openEditSecret = (
		secret: UserSecret,
		returnFocusElement?: HTMLElement | null,
	) => {
		secretDialogReturnFocusElement.current = returnFocusElement ?? null;
		setDialogState({ mode: "edit", open: true, secret });
	};
	const closeSecretDialog = () => {
		setDialogState((state) => ({ ...state, open: false }));
	};

	return (
		<div className="flex flex-col gap-6">
			<SettingsHeader
				actions={
					<div className="flex flex-wrap gap-2">
						<Button variant="outline" asChild>
							<a
								href={docs("/user-guides/user-secrets")}
								target="_blank"
								rel="noreferrer"
							>
								<SquareArrowOutUpRightIcon />
								Read the docs
							</a>
						</Button>
						<Button onClick={(event) => openAddSecret(event.currentTarget)}>
							<PlusIcon />
							Add secret
						</Button>
					</div>
				}
			>
				<SettingsHeaderTitle>Secrets</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					{filePathEnabled
						? "Secrets with an environment variable or file path are injected into workspaces you own when they start. Each environment variable and file path must be unique."
						: "Secrets with an environment variable are injected into workspaces you own when they start. Each environment variable must be unique. Your deployment administrator disabled file path secrets, so saved file paths stay stored but are not written to your workspaces, and they take effect again if file path secrets are enabled."}
				</SettingsHeaderDescription>
			</SettingsHeader>

			<SecretDialog
				open={dialogState.open}
				secret={dialogSecret}
				filePathEnabled={filePathEnabled}
				isSubmitting={isCreating || isUpdating}
				returnFocusElement={secretDialogReturnFocusElement.current}
				onClose={closeSecretDialog}
				onCreateSecret={onCreateSecret}
				onUpdateSecret={onUpdateSecret}
				onImportSecrets={onImportSecrets}
			/>

			{getSecretsError ? <ErrorAlert error={getSecretsError} /> : undefined}

			<section className="flex flex-col gap-4">
				<SecretsTable
					secrets={secrets}
					filePathEnabled={filePathEnabled}
					isLoading={isLoading}
					hasLoaded={hasLoadedSecrets}
					isDeleting={isDeleting}
					onAddSecret={openAddSecret}
					onEditSecret={openEditSecret}
					onDeleteSecret={onDeleteSecret}
					onToggleEnabled={onToggleSecretEnabled}
				/>
			</section>
		</div>
	);
};
