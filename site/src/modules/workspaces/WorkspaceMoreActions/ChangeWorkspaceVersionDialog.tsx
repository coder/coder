import AlertTitle from "@mui/material/AlertTitle";
import { templateVersions } from "api/queries/templates";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { DialogProps } from "components/Dialogs/Dialog";
import { FormFields } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { Pill } from "components/Pill/Pill";
import { InfoIcon } from "lucide-react";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { createDayString } from "utils/createDayString";

type ChangeWorkspaceVersionDialogProps = DialogProps & {
	workspace: Workspace;
	onClose: () => void;
	onConfirm: (version: TemplateVersion) => void;
};

export const ChangeWorkspaceVersionDialog: FC<
	ChangeWorkspaceVersionDialogProps
> = ({ workspace, onClose, onConfirm, ...dialogProps }) => {
	const { data: versions } = useQuery({
		...templateVersions(workspace.template_id),
		select: (data) => [...data].reverse(),
	});
	const currentVersion = versions?.find(
		(v) => workspace.latest_build.template_version_id === v.id,
	);
	const [newVersion, setNewVersion] = useState<TemplateVersion>();
	const validVersions = versions?.filter((v) => v.job.status === "succeeded");
	const selectedVersion = newVersion || currentVersion;

	return (
		<ConfirmDialog
			{...dialogProps}
			onClose={onClose}
			onConfirm={() => {
				if (newVersion) {
					onConfirm(newVersion);
				}
			}}
			hideCancel={false}
			type="success"
			cancelText="Cancel"
			confirmText="Change"
			title="Change version"
			description={
				<div className="flex flex-col gap-2">
					<p>You are about to change the version of this workspace.</p>
					{validVersions ? (
						<>
							<FormFields>
								<Autocomplete
									value={selectedVersion ?? null}
									onChange={(newTemplateVersion) => {
										setNewVersion(newTemplateVersion ?? undefined);
									}}
									options={validVersions}
									getOptionValue={(option) => option.id}
									getOptionLabel={(option) => option.name}
									isOptionEqualToValue={(option, value) =>
										option.id === value.id
									}
									renderOption={(option) => (
										<AvatarData
											avatar={
												<Avatar
													src={option.created_by.avatar_url}
													fallback={option.name}
												/>
											}
											title={
												<div className="flex flex-row items-center justify-between w-full">
													<div className="flex flex-row items-center gap-1">
														{option.name}
														{option.message && (
															<InfoIcon
																aria-hidden="true"
																className="size-icon-xs"
															/>
														)}
													</div>
													{workspace.template_active_version_id ===
														option.id && <Pill type="success">Active</Pill>}
												</div>
											}
											subtitle={createDayString(option.created_at)}
										/>
									)}
									placeholder="Template version name"
									clearable={false}
									loading={!versions}
									id="template-version-autocomplete"
								/>
							</FormFields>
							{selectedVersion && (
								<>
									{selectedVersion.message && (
										<TemplateUpdateMessage>
											{selectedVersion.message}
										</TemplateUpdateMessage>
									)}
									<Alert severity="info">
										<AlertTitle>
											Published by {selectedVersion.created_by.username}
										</AlertTitle>
									</Alert>
								</>
							)}
						</>
					) : (
						<Loader />
					)}
				</div>
			}
		/>
	);
};
