import { InfoIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useQuery } from "react-query";
import { templateVersions } from "#/api/queries/templates";
import type { TemplateVersion, Workspace } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { DialogProps } from "#/components/Dialogs/Dialog";
import type { SelectFilterOption } from "#/components/Filter/SelectFilter";
import { FormFields } from "#/components/Form/Form";
import { Loader } from "#/components/Loader/Loader";
import { Pill } from "#/components/Pill/Pill";
import { TemplateUpdateMessage } from "#/modules/templates/TemplateUpdateMessage";
import { cn } from "#/utils/cn";
import { createDayString } from "#/utils/createDayString";

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
	const [newVersion, setNewVersion] = useState<TemplateVersion>();
	const currentVersion = versions?.find(
		(v) => workspace.latest_build.template_version_id === v.id,
	);
	const validVersions = versions?.filter((v) => v.job.status === "succeeded");
	const selectedVersion = newVersion || currentVersion;

	const selectedOption: SelectFilterOption | undefined = useMemo(() => {
		if (!selectedVersion) {
			return undefined;
		}
		return {
			value: selectedVersion.id,
			label: selectedVersion.name,
			startIcon: (
				<Avatar
					size="sm"
					src={selectedVersion.created_by.avatar_url}
					fallback={selectedVersion.name}
				/>
			),
		};
	}, [selectedVersion]);

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
				<div className="flex flex-col gap-4">
					<p>You are about to change the version of this workspace.</p>
					{validVersions ? (
						<>
							<FormFields>
								<Combobox
									value={selectedVersion?.id}
									onValueChange={(id) => {
										if (!id) {
											// Ignore deselection; a version must
											// always be selected.
											return;
										}
										const next = validVersions.find((v) => v.id === id);
										setNewVersion(next);
									}}
								>
									<ComboboxTrigger asChild>
										<ComboboxButton
											id="template-version-autocomplete"
											aria-label="Template version"
											selectedOption={selectedOption}
											placeholder="Template version name"
											className="w-full min-w-0 pl-3.5"
										/>
									</ComboboxTrigger>
									<ComboboxContent
										className="max-w-none min-w-[min(100%,320px)]"
										align="start"
									>
										<ComboboxInput placeholder="Search versions…" />
										<ComboboxList>
											{validVersions.map((option) => (
												<ComboboxItem
													key={option.id}
													value={option.id}
													keywords={[option.name]}
													className={cn(
														"px-3 py-2 font-normal",
														"data-[selected=true]:bg-surface-tertiary",
													)}
												>
													<AvatarData
														avatar={
															<Avatar
																src={option.created_by.avatar_url}
																fallback={option.name}
															/>
														}
														title={
															<div className="flex w-full flex-row items-center justify-between gap-2">
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
																	option.id && (
																	<Pill type="success">Active</Pill>
																)}
															</div>
														}
														subtitle={createDayString(option.created_at)}
													/>
												</ComboboxItem>
											))}
										</ComboboxList>
										<ComboboxEmpty>No template versions found</ComboboxEmpty>
									</ComboboxContent>
								</Combobox>
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
