import { css } from "@emotion/css";
import AlertTitle from "@mui/material/AlertTitle";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import type { Template, TemplateVersion } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { DialogProps } from "components/Dialogs/Dialog";
import { FormFields } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { InfoIcon } from "lucide-react";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { type FC, useRef, useState } from "react";
import { createDayString } from "utils/createDayString";

export type ChangeVersionDialogProps = DialogProps & {
	template: Template | undefined;
	templateVersions: TemplateVersion[] | undefined;
	defaultTemplateVersion: TemplateVersion | undefined;
	onClose: () => void;
	onConfirm: (templateVersion: TemplateVersion) => void;
};

export const ChangeVersionDialog: FC<ChangeVersionDialogProps> = ({
	onConfirm,
	onClose,
	template,
	templateVersions,
	defaultTemplateVersion,
	...dialogProps
}) => {
	const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
	const selectedTemplateVersion = useRef<TemplateVersion | undefined>(
		defaultTemplateVersion,
	);
	const version = selectedTemplateVersion.current;
	const validTemplateVersions = templateVersions?.filter((version) => {
		return version.job.status === "succeeded";
	});

	return (
		<ConfirmDialog
			{...dialogProps}
			onClose={onClose}
			onConfirm={() => {
				if (selectedTemplateVersion.current) {
					onConfirm(selectedTemplateVersion.current);
				}
			}}
			hideCancel={false}
			type="success"
			cancelText="Cancel"
			confirmText="Change"
			title="Change version"
			description={
				<Stack>
					<p>You are about to change the version of this workspace.</p>
					{validTemplateVersions ? (
						<>
							<FormFields>
								<Autocomplete
									disableClearable
									options={validTemplateVersions}
									defaultValue={defaultTemplateVersion}
									id="template-version-autocomplete"
									open={isAutocompleteOpen}
									onChange={(_, newTemplateVersion) => {
										selectedTemplateVersion.current =
											newTemplateVersion ?? undefined;
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
													<Stack
														direction="row"
														justifyContent="space-between"
														style={{ width: "100%" }}
													>
														<Stack
															direction="row"
															alignItems="center"
															spacing={1}
														>
															{option.name}
															{option.message && (
																<InfoIcon css={{ width: 12, height: 12 }} />
															)}
														</Stack>
														{template?.active_version_id === option.id && (
															<Pill type="success">Active</Pill>
														)}
													</Stack>
												}
												subtitle={createDayString(option.created_at)}
											/>
										</li>
									)}
									renderInput={(params) => (
										<>
											<TextField
												{...params}
												fullWidth
												placeholder="Template version name"
												InputProps={{
													...params.InputProps,
													endAdornment: (
														<>
															{!templateVersions ? (
																<CircularProgress size={16} />
															) : null}
															{params.InputProps.endAdornment}
														</>
													),
													classes: { root: classNames.root },
												}}
											/>
										</>
									)}
								/>
							</FormFields>
							{version && (
								<>
									{version.message && (
										<TemplateUpdateMessage>
											{version.message}
										</TemplateUpdateMessage>
									)}
									<Alert severity="info">
										<AlertTitle>
											Published by {version.created_by.username}
										</AlertTitle>
									</Alert>
								</>
							)}
						</>
					) : (
						<Loader />
					)}
				</Stack>
			}
		/>
	);
};

const classNames = {
	// Same `padding-left` as input
	root: css`
    padding-left: 14px !important;
  `,
};
