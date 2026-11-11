import { CheckIcon, XIcon } from "lucide-react";
import { useMutation, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";
import type { SkillsScope } from "../utils/skills";

type SkillEnabledSwitchProps = {
	scope: SkillsScope;
	skill: SkillMetadata;
	readOnly: boolean;
};

export const SkillEnabledSwitch: React.FC<SkillEnabledSwitchProps> = ({
	scope,
	skill,
	readOnly,
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

	if (readOnly) {
		return skill.enabled ? (
			<CheckIcon
				role="img"
				aria-label="Enabled"
				className="size-icon-sm text-content-success"
			/>
		) : (
			<XIcon
				role="img"
				aria-label="Disabled"
				className="size-icon-sm text-content-secondary"
			/>
		);
	}

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
