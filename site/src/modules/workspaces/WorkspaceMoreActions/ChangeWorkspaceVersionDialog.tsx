import Autocomplete from "@mui/material/Autocomplete";
import TextField from "@mui/material/TextField";
import { templateVersions } from "api/queries/templates";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert, AlertTitle } from "components/Alert/Alert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { FormFields } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { Pill } from "components/Pill/Pill";
import { Spinner } from "components/Spinner/Spinner";
import { InfoIcon } from "lucide-react";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { createDayString } from "utils/createDayString";

type ChangeWorkspaceVersionDialogProps = {
	open: boolean;
	workspace: Workspace;
	onClose: () => void;
	onConfirm: (version: TemplateVersion) => void;
};

export const ChangeWorkspaceVersionDialog: FC<
	ChangeWorkspaceVersionDialogProps
> = ({ workspace, onClose, onConfirm, open }) => {
	const { data: versions } = useQuery({
		...templateVersions(workspace.template_id),
		select: (data) => [...data].reverse(),
	});
	const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
	const currentVersion = versions?.find(
		(v) => workspace.latest_build.template_version_id === v.id,
	);
	const [newVersion, setNewVersion] = useState<TemplateVersion>();
	const validVersions = versions?.filter((v) => v.job.status === "succeeded");
	const selectedVersion = newVersion || currentVersion;

	return (
		<ConfirmDialog
			open={open}
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
				<div className="flex flex-col gap-4">
					<p>You are about to change the version of this workspace.</p>
					{validVersions ? (
						<>
							<FormFields>
								<Autocomplete
									disableClearable
									options={validVersions}
									defaultValue={selectedVersion}
									id="template-version-autocomplete"
									open={isAutocompleteOpen}
									onChange={(_, newTemplateVersion) => {
										setNewVersion(newTemplateVersion);
									}}
									onOpen={() => {
										setIsAutocompleteOpen(true);
									}}
									onClose={() => {
										setIsAutocompleteOpen(false);
									}}
									isOptionEqualToValue={(
										option: TemplateVersion,
										value: TemplateVersion,
									) => option.id === value.id}
									getOptionLabel={(option) => option.name}
									renderOption={(props, option: TemplateVersion) => (
										<li {...props}>
											<AvatarData
												avatar={
													<Avatar
														src={option.created_by.avatar_url}
														fallback={option.name}
													/>
												}
												title={
													<div className="flex flex-row justify-between w-full">
														<div className="flex flex-row items-center gap-2">
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
										</li>
									)}
									renderInput={(params) => (
										<TextField
											{...params}
											fullWidth
											placeholder="Template version name"
											InputProps={{
												...params.InputProps,
												endAdornment: (
													<>
														{!versions && <Spinner loading size="sm" />}
														{params.InputProps.endAdornment}
													</>
												),
												sx: { pl: "14px !important" },
											}}
										/>
									)}
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
