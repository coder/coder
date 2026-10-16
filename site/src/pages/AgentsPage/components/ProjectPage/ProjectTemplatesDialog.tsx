import { useState } from "react";
import { useQuery } from "react-query";
import { isApiValidationError } from "#/api/errors";
import { agentTemplates } from "#/api/queries/templates";
import type { ChatProject, ChatProjectTemplates } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	ChatProjectTemplatesFields,
	chatProjectTemplatesRequest,
	chatProjectTemplatesValues,
	templatesValidationDetail,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";

type ProjectTemplatesDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSaving: boolean;
	readonly error: unknown;
	readonly onSave: (templates: ChatProjectTemplates) => void;
};

/** Edits the templates a project's chats may use, and their defaults. */
export const ProjectTemplatesDialog: React.FC<ProjectTemplatesDialogProps> = ({
	open,
	onOpenChange,
	...formProps
}) => {
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !formProps.isSaving) {
			onOpenChange(false);
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent aria-describedby={undefined}>
				<ProjectTemplatesForm
					{...formProps}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ProjectTemplatesFormProps = Omit<
	ProjectTemplatesDialogProps,
	"open" | "onOpenChange"
> & {
	readonly onCancel: () => void;
};

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	project,
	isSaving,
	error,
	onSave,
	onCancel,
}) => {
	// The dialog content mounts on open, so the draft starts from the
	// templates saved at that time and a project refetch does not reset it.
	const [initialValue] = useState(() =>
		chatProjectTemplatesValues(project.templates),
	);
	const [value, setValue] = useState(initialValue);
	const templatesQuery = useQuery(agentTemplates(project.organization_id));
	const eligibleTemplates = templatesQuery.data;
	const isUnchanged = JSON.stringify(value) === JSON.stringify(initialValue);
	const canSave = eligibleTemplates !== undefined && !isUnchanged && !isSaving;

	return (
		<form
			className="flex flex-col gap-4"
			onSubmit={(event) => {
				event.preventDefault();
				if (canSave) {
					onSave(chatProjectTemplatesRequest(value, eligibleTemplates));
				}
			}}
		>
			<DialogHeader>
				<DialogTitle>Edit templates</DialogTitle>
			</DialogHeader>
			<ChatProjectTemplatesFields
				templates={eligibleTemplates}
				templatesError={templatesQuery.error}
				saveError={templatesValidationDetail(error)}
				value={value}
				onChange={setValue}
				disabled={isSaving}
			/>
			{Boolean(error) && !isApiValidationError(error) && (
				<ErrorAlert error={error} showDebugDetail={false} />
			)}
			<DialogFooter>
				<Button
					type="button"
					variant="outline"
					disabled={isSaving}
					onClick={onCancel}
				>
					Cancel
				</Button>
				<Button type="submit" disabled={!canSave}>
					<Spinner loading={isSaving} />
					Save
				</Button>
			</DialogFooter>
		</form>
	);
};
