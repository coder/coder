import { useMutation, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { updateOrganizationSkillPreferences } from "#/api/queries/organizationSkills";
import type {
	OrganizationSkillPreferences,
	SkillMetadata,
} from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";

type OrganizationSkillPreferenceSwitchProps = {
	organizationId: string;
	skill: SkillMetadata;
	preferences: OrganizationSkillPreferences;
};

/** Locked, not cleared, while the master switch is off, so the choice survives. */
export const OrganizationSkillPreferenceSwitch: React.FC<
	OrganizationSkillPreferenceSwitchProps
> = ({ organizationId, skill, preferences }) => {
	const queryClient = useQueryClient();
	const toggleMutation = useMutation(
		updateOrganizationSkillPreferences(queryClient, organizationId),
	);

	return (
		<Switch
			aria-label={`Use ${skill.name} in my chats`}
			checked={
				toggleMutation.isPending
					? Boolean(toggleMutation.variables.skills?.[skill.id])
					: !preferences.disabled_skill_ids.includes(skill.id)
			}
			disabled={toggleMutation.isPending || preferences.all_skills_disabled}
			onCheckedChange={(enabled) =>
				toggleMutation.mutate(
					{ skills: { [skill.id]: enabled } },
					{
						onError: (error) => {
							toast.error(
								getErrorMessage(error, `Failed to update ${skill.name}.`),
								{ description: getErrorDetail(error) },
							);
						},
					},
				)
			}
		/>
	);
};
