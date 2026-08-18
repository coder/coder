import { type FC, useId, useState } from "react";
import {
	type ShareableWorkspaceOwners,
	ShareableWorkspaceOwnerses,
} from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { FormSection, HorizontalForm } from "#/components/Form/Form";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { DisableWorkspaceSharingDialog } from "./DisableWorkspaceSharingDialog";

const isShareableWorkspaceOwners = (
	value: string,
): value is ShareableWorkspaceOwners =>
	ShareableWorkspaceOwnerses.some((option) => option === value);

type WorkspaceSharingSectionProps = {
	organizationId: string;
	workspaceSharingGloballyDisabled?: boolean;
	shareableWorkspaceOwners: ShareableWorkspaceOwners;
	onChangeShareableOwners: (value: ShareableWorkspaceOwners) => void;
	isTogglingWorkspaceSharing: boolean;
};

export const WorkspaceSharingSection: FC<WorkspaceSharingSectionProps> = ({
	organizationId,
	workspaceSharingGloballyDisabled,
	shareableWorkspaceOwners,
	onChangeShareableOwners,
	isTogglingWorkspaceSharing,
}) => {
	const [pendingSharingChange, setPendingSharingChange] =
		useState<ShareableWorkspaceOwners | null>(null);

	const id = useId();
	const workspaceSharingId = `${id}-workspace-sharing`;
	const sharingServiceAccountsId = `${id}-sharing-service-accounts`;
	const sharingEveryoneId = `${id}-sharing-everyone`;

	return (
		<>
			<HorizontalForm className="mt-12">
				<FormSection
					title="Workspace Sharing"
					description="Control whether workspace owners can share their workspaces."
				>
					<div className="flex flex-col gap-2">
						{workspaceSharingGloballyDisabled && (
							<Alert severity="warning" className="mb-4">
								<AlertTitle>Disabled by deployment settings</AlertTitle>
								Workspace sharing has been disallowed by an administrator.
								Sharing must be allowed by an administrator before sharing can
								be used in this organization.
							</Alert>
						)}
						<div className="flex items-start gap-3">
							<Checkbox
								id={workspaceSharingId}
								checked={
									!workspaceSharingGloballyDisabled &&
									shareableWorkspaceOwners !== "none"
								}
								disabled={
									workspaceSharingGloballyDisabled || isTogglingWorkspaceSharing
								}
								onCheckedChange={(checked) => {
									if (checked) {
										onChangeShareableOwners("service_accounts");
									} else {
										setPendingSharingChange("none");
									}
								}}
							/>
							<div className="flex flex-col gap-3">
								<div className="flex flex-col">
									<label
										htmlFor={workspaceSharingId}
										className="text-sm cursor-pointer"
									>
										Allow workspace sharing
									</label>
									<div className="text-xs text-content-secondary">
										When enabled, workspace owners can share their workspaces
										with other users in this organization.
									</div>
								</div>
								{shareableWorkspaceOwners !== "none" &&
									!workspaceSharingGloballyDisabled && (
										<RadioGroup
											value={shareableWorkspaceOwners}
											onValueChange={(value) => {
												if (!isShareableWorkspaceOwners(value)) {
													return;
												}
												// Restricting from everyone to service accounts
												// revokes existing shares, so confirm first.
												if (
													shareableWorkspaceOwners === "everyone" &&
													value === "service_accounts"
												) {
													setPendingSharingChange("service_accounts");
												} else {
													onChangeShareableOwners(value);
												}
											}}
											disabled={isTogglingWorkspaceSharing}
											className="ml-1"
										>
											<div className="flex items-start gap-2">
												<RadioGroupItem
													value="service_accounts"
													id={sharingServiceAccountsId}
													className="mt-0.5"
												/>
												<div className="flex flex-col">
													<label
														htmlFor={sharingServiceAccountsId}
														className="text-sm cursor-pointer"
													>
														Only service accounts can share workspaces
													</label>
													<span className="text-xs text-content-secondary">
														Service accounts are non-login accounts typically
														used for automation, CI/CD pipelines, and
														centrally-managed shared environments.
													</span>
												</div>
											</div>
											<div className="flex items-center gap-2">
												<RadioGroupItem
													value="everyone"
													id={sharingEveryoneId}
												/>
												<label
													htmlFor={sharingEveryoneId}
													className="text-sm cursor-pointer"
												>
													All members can share workspaces
												</label>
											</div>
										</RadioGroup>
									)}
							</div>
						</div>
					</div>
				</FormSection>
			</HorizontalForm>

			<DisableWorkspaceSharingDialog
				isOpen={pendingSharingChange !== null}
				organizationId={organizationId}
				newSetting={pendingSharingChange ?? "none"}
				onConfirm={async () => {
					if (pendingSharingChange !== null) {
						await onChangeShareableOwners(pendingSharingChange);
					}
					setPendingSharingChange(null);
				}}
				onCancel={() => setPendingSharingChange(null)}
				isLoading={isTogglingWorkspaceSharing}
			/>
		</>
	);
};
