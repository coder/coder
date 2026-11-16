import { useId } from "react";
import { useMutation, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { updateOrganizationSkillPreferences } from "#/api/queries/organizationSkills";
import type { OrganizationSkillPreferences } from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";

type OrganizationSkillsMasterSwitchProps = {
	organizationId: string;
	preferences: OrganizationSkillPreferences;
};

export const OrganizationSkillsMasterSwitch: React.FC<
	OrganizationSkillsMasterSwitchProps
> = ({ organizationId, preferences }) => {
	const labelId = useId();
	const descriptionId = useId();

	const queryClient = useQueryClient();
	const toggleMutation = useMutation(
		updateOrganizationSkillPreferences(queryClient, organizationId),
	);

	const allSkillsDisabled = toggleMutation.isPending
		? toggleMutation.variables.all_skills_disabled
		: preferences.all_skills_disabled;

	return (
		<div className="flex flex-col gap-2">
			<h4
				id={labelId}
				className="m-0 text-sm font-semibold text-content-primary"
			>
				Use organization skills in my chats
			</h4>
			<div className="flex items-center justify-between gap-4">
				<p
					id={descriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Turn this off to hide every skill from this organization in your
					chats, including skills added later. Your choices for individual
					skills are kept and apply again when you turn this back on.
				</p>
				<Switch
					aria-labelledby={labelId}
					aria-describedby={descriptionId}
					checked={!allSkillsDisabled}
					disabled={toggleMutation.isPending}
					onCheckedChange={(checked) =>
						toggleMutation.mutate(
							{ all_skills_disabled: !checked },
							{
								onError: (error) => {
									toast.error(
										getErrorMessage(
											error,
											"Failed to update your organization skills.",
										),
										{ description: getErrorDetail(error) },
									);
								},
							},
						)
					}
				/>
			</div>
		</div>
	);
};
