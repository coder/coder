import { useMutation, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";
import type { SkillsScope } from "../utils/skills";

type SkillEnabledSwitchProps = {
	scope: SkillsScope;
	skill: SkillMetadata;
};

export const SkillEnabledSwitch: React.FC<SkillEnabledSwitchProps> = ({
	scope,
	skill,
}) => {
	const queryClient = useQueryClient();
	const toggleMutation = useMutation({
		...scope.update(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to update ${skill.name}.`), {
				description: getErrorDetail(error),
			});
		},
	});

	return (
		<Switch
			aria-label={`Enable ${skill.name}`}
			checked={
				toggleMutation.isPending
					? Boolean(toggleMutation.variables.req.enabled)
					: skill.enabled
			}
			disabled={toggleMutation.isPending}
			onCheckedChange={(enabled) =>
				toggleMutation.mutate({ name: skill.name, req: { enabled } })
			}
		/>
	);
};
