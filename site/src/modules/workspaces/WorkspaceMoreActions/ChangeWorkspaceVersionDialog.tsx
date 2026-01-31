import { templateVersions } from "api/queries/templates";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert, AlertTitle } from "components/Alert/Alert";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Loader } from "components/Loader/Loader";
import { Pill } from "components/Pill/Pill";
import { InfoIcon } from "lucide-react";
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
> = ({ open, workspace, onClose, onConfirm }) => {
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
	const dialogContentRef = useRef<HTMLDivElement>(null);

	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent ref={dialogContentRef}>
				<DialogHeader>
					<DialogTitle>Change version</DialogTitle>
					<DialogDescription>
						You are about to change the version of this workspace.
					</DialogDescription>
				</DialogHeader>
				{validVersions ? (
					<div className="flex flex-col gap-4">
						<Autocomplete
							value={selectedVersion ?? null}
							onChange={(newTemplateVersion) => {
								setNewVersion(newTemplateVersion ?? undefined);
							}}
							options={validVersions}
							getOptionValue={(option) => option.id}
							getOptionLabel={(option) => option.name}
							isOptionEqualToValue={(option, value) => option.id === value.id}
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
											{workspace.template_active_version_id === option.id && (
												<Pill type="success">Active</Pill>
											)}
										</div>
									}
									subtitle={createDayString(option.created_at)}
								/>
							)}
							placeholder="Template version name"
							clearable={false}
							loading={!versions}
							id="template-version-autocomplete"
							popoverContainer={dialogContentRef.current}
						/>
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
					</div>
				) : (
					<Loader />
				)}
				<DialogFooter>
					<Button variant="outline" onClick={onClose}>
						Cancel
					</Button>
					<Button
						disabled={!newVersion}
						onClick={() => {
							if (newVersion) {
								onConfirm(newVersion);
							}
						}}
					>
						Change
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
