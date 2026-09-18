import { useFormik } from "formik";
import { type FC, type ReactNode, useState } from "react";
import { flushSync } from "react-dom";
import type * as TypesGen from "#/api/typesGenerated";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { MCPServerFormDialogs } from "./MCPServerFormDialogs";
import { MCPServerFormFields } from "./MCPServerFormFields";
import { MCPServerFormHeader } from "./MCPServerFormHeader";
import { MCPServerSharingDialog } from "./MCPServerSharingDialog";
import {
	buildCreateMCPServerConfigRequest,
	buildInitialMCPServerFormValues,
	buildUpdateMCPServerConfigRequest,
	canSubmitMCPServerForm,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

export type MCPServerFormSaveResult = {
	afterSave?: () => void;
};

type MCPServerFormCreateProps = {
	server?: undefined;
	// Create-only callers cannot open the server list, so the back link and
	// cancel action are omitted rather than pointing at a denied page.
	listPath?: string;
	isSaving: boolean;
	isDeleting?: false;
	isRegeneratingSigningSecret?: false;
	canSelectUserOIDC: boolean;
	organizationPicker?: ReactNode;
	canShareServer?: false;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<MCPServerFormSaveResult | undefined>;
	onUpdateServer?: undefined;
	onDeleteServer?: undefined;
	onRegenerateSigningSecret?: undefined;
	onToggleEnabled?: undefined;
	onCancel?: () => void;
};

type MCPServerFormEditProps = {
	server: TypesGen.MCPServerConfig;
	listPath: string;
	isSaving: boolean;
	isDeleting: boolean;
	isRegeneratingSigningSecret: boolean;
	canSelectUserOIDC: boolean;
	organizationPicker?: ReactNode;
	canShareServer?: boolean;
	onCreateServer?: undefined;
	onUpdateServer?: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<MCPServerFormSaveResult | undefined>;
	onDeleteServer?: (serverId: string) => Promise<void>;
	onRegenerateSigningSecret?: () => void;
	onToggleEnabled?: (enabled: boolean) => void;
	onCancel: () => void;
};

type MCPServerFormProps = MCPServerFormCreateProps | MCPServerFormEditProps;

export const MCPServerForm: FC<MCPServerFormProps> = ({
	server,
	listPath,
	isSaving,
	isDeleting = false,
	isRegeneratingSigningSecret = false,
	canSelectUserOIDC,
	organizationPicker,
	canShareServer = false,
	onCreateServer,
	onUpdateServer,
	onDeleteServer,
	onRegenerateSigningSecret,
	onToggleEnabled,
	onCancel,
}) => {
	const isEditing = server !== undefined;

	const [showDetails, setShowDetails] = useState(false);
	const [showAuth, setShowAuth] = useState(false);
	const [showBehavior, setShowBehavior] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);
	const [sharingOpen, setSharingOpen] = useState(false);
	const [
		confirmingRegenerateSigningSecret,
		setConfirmingRegenerateSigningSecret,
	] = useState(false);

	const form = useFormik<MCPServerFormValues>({
		initialValues: buildInitialMCPServerFormValues(server),
		onSubmit: async (values, helpers) => {
			if (isSaving) return;
			let result: MCPServerFormSaveResult | undefined;
			if (server && onUpdateServer) {
				result = await onUpdateServer(
					server.id,
					buildUpdateMCPServerConfigRequest(values),
				);
			} else if (onCreateServer) {
				result = await onCreateServer(
					buildCreateMCPServerConfigRequest(values),
				);
			}
			if (!result) return;
			// Commit the clean baseline before deferred navigation so the route
			// blocker does not observe stale dirty state.
			flushSync(() => {
				if (isEditing) {
					helpers.resetForm({ values });
				} else {
					helpers.resetForm();
				}
			});
			result.afterSave?.();
		},
	});

	const isDisabled = isSaving || isDeleting || isRegeneratingSigningSecret;
	const areFieldsDisabled =
		isDisabled || (isEditing && onUpdateServer === undefined);
	// Editing requires a change before submitting, matching the provider form.
	const canSubmit =
		canSubmitMCPServerForm(form.values, areFieldsDisabled) &&
		(!isEditing || form.dirty);
	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);
	const title = isEditing
		? form.values.displayName || "Edit server"
		: "Add server";

	return (
		<>
			<MCPServerFormHeader
				server={server}
				title={title}
				iconUrl={form.values.iconURL}
				listPath={listPath}
				isEditing={isEditing}
				isDisabled={isDisabled}
				onRequestDelete={
					onDeleteServer ? () => setConfirmingDelete(true) : undefined
				}
				onRequestRegenerateSigningSecret={
					onRegenerateSigningSecret
						? () => setConfirmingRegenerateSigningSecret(true)
						: undefined
				}
				onShareServer={canShareServer ? () => setSharingOpen(true) : undefined}
				onToggleEnabled={onToggleEnabled}
			/>
			<div className="flex flex-col gap-6 pt-6">
				<MCPServerFormFields
					form={form}
					isSaving={isSaving}
					isDisabled={areFieldsDisabled}
					canSubmit={canSubmit}
					isEditing={isEditing}
					canSelectUserOIDC={canSelectUserOIDC}
					organizationPicker={organizationPicker}
					onCancel={onCancel}
					showDetails={showDetails}
					setShowDetails={setShowDetails}
					showAuth={showAuth}
					setShowAuth={setShowAuth}
					showBehavior={showBehavior}
					setShowBehavior={setShowBehavior}
				/>
			</div>
			{server && canShareServer && (
				<MCPServerSharingDialog
					open={sharingOpen}
					onOpenChange={setSharingOpen}
					organizationId={server.organization_id}
					serverId={server.id}
					serverName={server.display_name || server.slug}
				/>
			)}
			<MCPServerFormDialogs
				server={server}
				confirmingDelete={confirmingDelete}
				setConfirmingDelete={setConfirmingDelete}
				onDeleteServer={onDeleteServer}
				isDeleting={isDeleting}
				confirmingRegenerateSigningSecret={confirmingRegenerateSigningSecret}
				setConfirmingRegenerateSigningSecret={
					setConfirmingRegenerateSigningSecret
				}
				onRegenerateSigningSecret={onRegenerateSigningSecret}
				unsavedChanges={unsavedChanges}
			/>
		</>
	);
};
