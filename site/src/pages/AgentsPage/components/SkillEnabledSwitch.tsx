import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { type SkillOwner, skillList, updateSkill } from "#/api/queries/skills";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";

type SkillEnabledSwitchProps = {
	owner: SkillOwner;
	skill: SkillMetadata;
	disabled: boolean;
};

export const SkillEnabledSwitch: React.FC<SkillEnabledSwitchProps> = ({
	owner,
	skill,
	disabled,
}) => {
	const queryClient = useQueryClient();
	const toggleMutation = useMutation({
		...updateSkill(queryClient, owner),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to update ${skill.name}.`), {
				description: getErrorDetail(error),
			});
		},
		// Refetching also drops a skill that was deleted elsewhere.
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: skillList(owner).queryKey,
				exact: true,
			}),
	});

	return (
		<Switch
			aria-label={`Enable ${skill.name}`}
			checked={
				toggleMutation.isPending
					? Boolean(toggleMutation.variables.req.enabled)
					: skill.enabled
			}
			disabled={disabled || toggleMutation.isPending}
			onCheckedChange={(enabled) =>
				toggleMutation.mutate({ name: skill.name, req: { enabled } })
			}
		/>
	);
};
