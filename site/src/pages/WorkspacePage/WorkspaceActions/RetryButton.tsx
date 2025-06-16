import type { Workspace } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import { RotateCcwIcon } from "lucide-react";
import type { FC } from "react";
import { ConditionalBuildParametersPopover } from "./ConditionalBuildParametersPopover";
import type { ActionButtonProps } from "./Buttons";

type RetryButtonProps = Omit<ActionButtonProps, "loading"> & {
	enableBuildParameters: boolean;
	workspace: Workspace;
};

export const RetryButton: FC<RetryButtonProps> = ({
	handleAction,
	workspace,
	enableBuildParameters,
}) => {
	const mainAction = (
		<TopbarButton onClick={() => handleAction()}>
			<RotateCcwIcon />
			Retry
		</TopbarButton>
	);

	if (!enableBuildParameters) {
		return mainAction;
	}

	return (
		<div className="flex gap-1 items-center">
			{mainAction}
			<ConditionalBuildParametersPopover
				label="Retry with build parameters"
				workspace={workspace}
				onSubmit={handleAction}
			/>
		</div>
	);
};
