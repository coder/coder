import type { ComponentProps, FC } from "react";
import type { Workspace } from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm";

type WorkspaceSettingsPageViewProps = {
	error: unknown;
	workspace: Workspace;
	onCancel: () => void;
	onSubmit: ComponentProps<typeof WorkspaceSettingsForm>["onSubmit"];
};

export const WorkspaceSettingsPageView: FC<WorkspaceSettingsPageViewProps> = ({
	onCancel,
	onSubmit,
	error,
	workspace,
}) => {
	return (
		<div className="flex flex-col gap-12">
			<SettingsHeader>
				<SettingsHeaderTitle>General</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Update the name and automatic update behavior for this workspace.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<WorkspaceSettingsForm
				error={error}
				workspace={workspace}
				onCancel={onCancel}
				onSubmit={onSubmit}
			/>
		</div>
	);
};
