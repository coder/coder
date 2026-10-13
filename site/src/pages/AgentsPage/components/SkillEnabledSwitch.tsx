import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import type { SkillsScope } from "../utils/skills";

type SkillEnabledSwitchProps = {
	scope: SkillsScope;
	skill: SkillMetadata;
	disabled: boolean;
};

export const SkillEnabledSwitch: React.FC<SkillEnabledSwitchProps> = ({
	scope,
	skill,
	disabled,
}) => {
	const queryClient = useQueryClient();
	const toggleMutation = useMutation({
		...scope.update(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to update ${skill.name}.`), {
				description: getErrorDetail(error),
			});
		},
		// Refetching also drops a skill that was deleted elsewhere.
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: scope.list.queryKey,
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
