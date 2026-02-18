import { templateVersions } from "api/queries/templates";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "components/Combobox/Combobox";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Loader } from "components/Loader/Loader";
import { ChevronDownIcon, InfoIcon, UserIcon } from "lucide-react";
import { TemplateUpdateMessage } from "modules/templates/TemplateUpdateMessage";
import { type FC, useRef, useState } from "react";
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
	const currentVersion = versions?.find(
		(v) => workspace.latest_build.template_version_id === v.id,
	);
	const [newVersion, setNewVersion] = useState<TemplateVersion>();
	const validVersions = versions?.filter((v) => v.job.status === "succeeded");
	const selectedVersion = newVersion || currentVersion;
	const popoverContainerRef = useRef<HTMLDivElement>(null);

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
							<div ref={popoverContainerRef}>
								<Combobox
									value={selectedVersion?.id}
									onValueChange={(id) => {
										const version = validVersions.find((v) => v.id === id);
										if (version) {
											setNewVersion(version);
										}
									}}
								>
									<ComboboxTrigger asChild>
										<Button
											variant="outline"
											className="h-16 w-full justify-between"
										>
											{selectedVersion ? (
												<div className="text-left justify-between flex-1">
													<AvatarData
														avatar={
															<Avatar
																src={selectedVersion.created_by.avatar_url}
																fallback={selectedVersion.name}
															/>
														}
														title={
															<div className="flex flex-row justify-between w-full gap-2">
																<div className="flex flex-row items-center gap-2">
																	{selectedVersion.name}
																	{selectedVersion.message && (
																		<InfoIcon
																			aria-hidden="true"
																			className="size-icon-xs"
																		/>
																	)}
																</div>
																{workspace.template_active_version_id ===
																	selectedVersion.id && (
																	<Badge variant="green">Active</Badge>
																)}
															</div>
														}
														subtitle={createDayString(
															selectedVersion.created_at,
														)}
													/>
												</div>
											) : null}
											<ChevronDownIcon />
										</Button>
									</ComboboxTrigger>
									<ComboboxContent
										className="w-[var(--radix-popper-anchor-width)]"
										container={popoverContainerRef.current}
									>
										<ComboboxInput placeholder="Search versions..." />
										<ComboboxList>
											{validVersions.map((version) => (
												<ComboboxItem
													key={version.id}
													value={version.id}
													keywords={[version.name]}
												>
													<AvatarData
														avatar={
															<Avatar
																src={version.created_by.avatar_url}
																fallback={version.name}
															/>
														}
														title={
															<div className="flex flex-row justify-between w-full gap-2">
																<div className="flex flex-row items-center gap-2">
																	{version.name}
																	{version.message && (
																		<InfoIcon
																			aria-hidden="true"
																			className="size-icon-xs"
																		/>
																	)}
																</div>
																{workspace.template_active_version_id ===
																	version.id && (
																	<Badge variant="green">Active</Badge>
																)}
															</div>
														}
														subtitle={createDayString(version.created_at)}
													/>
												</ComboboxItem>
											))}
										</ComboboxList>
										<ComboboxEmpty>No versions found</ComboboxEmpty>
									</ComboboxContent>
								</Combobox>
							</div>
							{selectedVersion && (
								<>
									{selectedVersion.message && (
										<TemplateUpdateMessage>
											{selectedVersion.message}
										</TemplateUpdateMessage>
									)}
									<div className="flex items-center gap-1 font-normal text-sm">
										<UserIcon className="size-icon-sm" />
										<span>
											Published by {selectedVersion.created_by.username}
										</span>
									</div>
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
