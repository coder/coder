import { useFormik } from "formik";
import { type FC, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { MCPServerFormDialogs } from "./MCPServerFormDialogs";
import { MCPServerFormFields } from "./MCPServerFormFields";
import { MCPServerFormHeader } from "./MCPServerFormHeader";
import {
	buildCreateRequest,
	buildInitialValues,
	buildUpdateRequest,
	canSubmitMCPServerForm,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

interface MCPServerFormProps {
	server?: TypesGen.MCPServerConfig;
	isSaving: boolean;
	isDeleting: boolean;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onUpdateServer: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<unknown>;
	onDeleteServer?: (serverId: string) => Promise<void>;
	onToggleEnabled?: (enabled: boolean) => void;
	onCancel: () => void;
}

export const MCPServerForm: FC<MCPServerFormProps> = ({
	server,
	isSaving,
	isDeleting,
	onCreateServer,
	onUpdateServer,
	onDeleteServer,
	onToggleEnabled,
	onCancel,
}) => {
	const isEditing = Boolean(server);
	const [showDetails, setShowDetails] = useState(false);
	const [showAuth, setShowAuth] = useState(false);
	const [showBehavior, setShowBehavior] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const form = useFormik<MCPServerFormValues>({
		initialValues: buildInitialValues(server),
		onSubmit: async (values) => {
			if (isSaving) return;
			if (server) {
				await onUpdateServer(server.id, buildUpdateRequest(values));
			} else {
				await onCreateServer(buildCreateRequest(values));
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
				isSaving={isSaving}
				onRequestDelete={() => setConfirmingDelete(true)}
				onToggleEnabled={onToggleEnabled}
			/>
			<div className="flex flex-col gap-6 pt-6">
				<MCPServerFormFields
					form={form}
					isSaving={isSaving}
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
