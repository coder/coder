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

	return (
		<div className="flex flex-col gap-4">
			<SectionHeader
				level="section"
				label={
					<span id={labelId}>Members can turn off organization skills</span>
				}
				description="Each member can turn off individual organization skills, or all of them, for their own chats."
				action={
					settingsQuery.data ? (
						<Switch
							aria-labelledby={labelId}
							checked={
								updateMutation.isPending
									? updateMutation.variables.members_can_disable_skills
									: settingsQuery.data.members_can_disable_skills
							}
							disabled={!canUpdate || updateMutation.isPending}
							onCheckedChange={(checked) =>
								updateMutation.mutate({ members_can_disable_skills: checked })
							}
						/>
					) : (
						!settingsQuery.isError && <Skeleton className="h-5 w-9" />
					)
				}
			/>
			{settingsQuery.isError && !settingsQuery.data && (
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
