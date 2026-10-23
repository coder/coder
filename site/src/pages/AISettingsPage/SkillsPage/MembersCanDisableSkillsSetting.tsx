import { useId } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	organizationSkillSettings,
	updateOrganizationSkillSettings,
} from "#/api/queries/organizationSkills";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";
import { EnabledStateIcon } from "#/pages/AgentsPage/components/EnabledStateIcon";
import { RetryButton } from "#/pages/AgentsPage/components/RetryButton";
import { SectionHeader } from "#/pages/AgentsPage/components/SectionHeader";

type MembersCanDisableSkillsSettingProps = {
	organizationId: string;
	canUpdate: boolean;
};

/** Read-only for users who cannot update organization skills. */
export const MembersCanDisableSkillsSetting: React.FC<
	MembersCanDisableSkillsSettingProps
> = ({ organizationId, canUpdate }) => {
	const labelId = useId();
	const descriptionId = useId();
	const queryClient = useQueryClient();
	const settingsQuery = useQuery(organizationSkillSettings(organizationId));
	const updateMutation = useMutation({
		...updateOrganizationSkillSettings(queryClient, organizationId),
		onError: (error) => {
			toast.error(
				getErrorMessage(error, "Failed to update the organization setting."),
				{ description: getErrorDetail(error) },
			);
		},
	});

	let action: React.ReactNode;
	if (!settingsQuery.data) {
		action = !settingsQuery.isError && <Skeleton className="h-5 w-9" />;
	} else if (!canUpdate) {
		action = (
			<EnabledStateIcon
				enabled={settingsQuery.data.members_can_disable_skills}
			/>
		);
	} else {
		action = (
			<Switch
				aria-labelledby={labelId}
				aria-describedby={descriptionId}
				checked={
					updateMutation.isPending
						? updateMutation.variables.members_can_disable_skills
						: settingsQuery.data.members_can_disable_skills
				}
				disabled={updateMutation.isPending}
				onCheckedChange={(checked) =>
					updateMutation.mutate({ members_can_disable_skills: checked })
				}
			/>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<SectionHeader
				level="section"
				label={
					<span id={labelId}>Members can turn off organization skills</span>
				}
				description="Each member can turn off individual organization skills, or all of them, for their own chats."
				descriptionId={descriptionId}
				action={action}
			/>
			{settingsQuery.isError && (
				<ErrorAlert
					error={settingsQuery.error}
					actions={
						<RetryButton
							size="sm"
							variant="outline"
							aria-label="Retry loading the organization setting"
							isRetrying={settingsQuery.isFetching}
							onRetry={() => {
								void settingsQuery.refetch();
							}}
						/>
					}
				/>
			)}
		</div>
	);
};
