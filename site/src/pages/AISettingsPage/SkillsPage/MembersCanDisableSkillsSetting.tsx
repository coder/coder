import { useId } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	organizationSkillSettings,
	updateOrganizationSkillSettings,
} from "#/api/queries/organizationSkills";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";
import { RetryButton } from "#/pages/AgentsPage/components/RetryButton";

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

	return (
		<div className="flex flex-col gap-2">
			<h3
				id={labelId}
				className="m-0 text-sm font-semibold text-content-primary"
			>
				Members can turn off organization skills
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p
					id={descriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Each member can turn off individual organization skills, or all of
					them, for their own chats.
					{!canUpdate &&
						" Only users who can update organization skills can change this setting."}
				</p>
				{settingsQuery.data ? (
					<Switch
						aria-labelledby={labelId}
						aria-describedby={descriptionId}
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
				) : settingsQuery.isError ? (
					<RetryButton
						size="sm"
						variant="outline"
						aria-label="Retry loading the organization setting"
						isRetrying={settingsQuery.isFetching}
						onRetry={() => {
							void settingsQuery.refetch();
						}}
					/>
				) : (
					<Skeleton className="h-5 w-9" />
				)}
			</div>
			{settingsQuery.isError && !settingsQuery.data && (
				<p role="alert" className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						settingsQuery.error,
						"Failed to load the organization setting.",
					)}
				</p>
			)}
		</div>
	);
};
