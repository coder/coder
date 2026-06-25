import { useFormik } from "formik";
import { type FC, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { MCPServerFormDialogs } from "./MCPServerFormDialogs";
import { MCPServerFormFields } from "./MCPServerFormFields";
import { MCPServerFormHeader } from "./MCPServerFormHeader";
import {
	buildCreateMCPServerConfigRequest,
	buildInitialMCPServerFormValues,
	buildUpdateMCPServerConfigRequest,
	canSubmitMCPServerForm,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

type MCPServerFormCreateProps = {
	server?: undefined;
	isSaving: boolean;
	isDeleting?: false;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onUpdateServer?: undefined;
	onDeleteServer?: undefined;
	onToggleEnabled?: undefined;
	onCancel: () => void;
};

type MCPServerFormEditProps = {
	server: TypesGen.MCPServerConfig;
	isSaving: boolean;
	isDeleting: boolean;
	onCreateServer?: undefined;
	onUpdateServer: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<unknown>;
	onDeleteServer?: (serverId: string) => Promise<void>;
	onToggleEnabled?: (enabled: boolean) => void;
	onCancel: () => void;
};

type MCPServerFormProps = MCPServerFormCreateProps | MCPServerFormEditProps;

export const MCPServerForm: FC<MCPServerFormProps> = ({
	server,
	isSaving,
	isDeleting = false,
	onCreateServer,
	onUpdateServer,
	onDeleteServer,
	onToggleEnabled,
	onCancel,
}) => {
	const isEditing = server !== undefined;

	const [showDetails, setShowDetails] = useState(false);
	const [showAuth, setShowAuth] = useState(false);
	const [showBehavior, setShowBehavior] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const form = useFormik<MCPServerFormValues>({
		initialValues: buildInitialMCPServerFormValues(server),
		onSubmit: async (values) => {
			if (isSaving) return;
			if (server && onUpdateServer) {
				await onUpdateServer(
					server.id,
					buildUpdateMCPServerConfigRequest(values),
				);
			} else if (onCreateServer) {
				await onCreateServer(buildCreateMCPServerConfigRequest(values));
			}
		},
	});

	const isDisabled = isSaving || isDeleting;
	const canSubmit = canSubmitMCPServerForm(form.values, isDisabled);
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
				isEditing={isEditing}
				isDisabled={isDisabled}
				onRequestDelete={() => setConfirmingDelete(true)}
				onToggleEnabled={onToggleEnabled}
			/>
			<div className="flex flex-col gap-6 pt-6">
				<MCPServerFormFields
					form={form}
					isSaving={isSaving}
					isDisabled={isDisabled}
					canSubmit={canSubmit}
					isEditing={isEditing}
					onCancel={onCancel}
					showDetails={showDetails}
					setShowDetails={setShowDetails}
					showAuth={showAuth}
					setShowAuth={setShowAuth}
					showBehavior={showBehavior}
					setShowBehavior={setShowBehavior}
				/>
			</div>
			<MCPServerFormDialogs
				server={server}
				confirmingDelete={confirmingDelete}
				setConfirmingDelete={setConfirmingDelete}
				onDeleteServer={onDeleteServer}
				isDeleting={isDeleting}
				unsavedChanges={unsavedChanges}
			/>
		</>
	);
};
