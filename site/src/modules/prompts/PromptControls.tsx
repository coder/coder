import type { FC, ReactNode } from "react";

type PromptControlsProps = {
	leftActions: ReactNode;
	rightActions: ReactNode;
	statusMessages?: ReactNode;
};

export const PromptControls: FC<PromptControlsProps> = ({
	leftActions,
	rightActions,
	statusMessages,
}) => {
	return (
		<>
			<div className="flex items-center justify-between pt-2 gap-2">
				<div className="flex items-center gap-1 flex-1 min-w-0">
					{leftActions}
				</div>
				<div className="flex items-center gap-2">{rightActions}</div>
			</div>
			{statusMessages}
		</>
	);
};
