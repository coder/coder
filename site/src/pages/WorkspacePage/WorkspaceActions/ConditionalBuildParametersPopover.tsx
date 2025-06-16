import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import visuallyHidden from "@mui/utils/visuallyHidden";
import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { useState } from "react";
import { BuildParametersPopover } from "./BuildParametersPopover";
import { UpdateBuildParametersDialogExperimental } from "modules/workspaces/WorkspaceMoreActions/UpdateBuildParametersDialogExperimental";
import { useQuery } from "react-query";
import { API } from "api/api";

interface ConditionalBuildParametersPopoverProps {
	workspace: Workspace;
	disabled?: boolean;
	onSubmit: (buildParameters?: WorkspaceBuildParameter[]) => void;
	label: string;
}

export const ConditionalBuildParametersPopover: FC<ConditionalBuildParametersPopoverProps> = ({
	workspace,
	disabled,
	label,
	onSubmit,
}) => {
	const [isDialogOpen, setIsDialogOpen] = useState(false);

	const { data: parameters } = useQuery({
		queryKey: ["workspace", workspace.id, "parameters"],
		queryFn: () => API.getWorkspaceParameters(workspace),
	});
	
	const ephemeralParameters = parameters
		? parameters.templateVersionRichParameters.filter((p) => p.ephemeral)
		: [];

	// If using classic parameter flow, render the original BuildParametersPopover
	if (workspace.template_use_classic_parameter_flow) {
		return (
			<BuildParametersPopover
				workspace={workspace}
				disabled={disabled}
				label={label}
				onSubmit={onSubmit}
			/>
		);
	}

	// For experimental flow, show dialog directing to workspace settings
	const handleClick = () => {
		if (ephemeralParameters.length > 0) {
			setIsDialogOpen(true);
		} else {
			// If no ephemeral parameters, proceed with the action
			onSubmit();
		}
	};

	return (
		<>
			<TopbarButton
				data-testid="build-parameters-button"
				disabled={disabled}
				className="min-w-fit"
				onClick={handleClick}
			>
				<ChevronDownIcon />
				<span css={{ ...visuallyHidden }}>{label}</span>
			</TopbarButton>

			<UpdateBuildParametersDialogExperimental
				open={isDialogOpen}
				onClose={() => setIsDialogOpen(false)}
				missedParameters={ephemeralParameters}
				workspaceOwnerName={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
			/>
		</>
	);
};