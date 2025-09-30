import MiniTooltip from "components/MiniTooltip/MiniTooltip";
import { TooltipContentProps } from "components/Tooltip/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import type { FC, HTMLAttributes } from "react";

interface CopyableValueProps extends HTMLAttributes<HTMLSpanElement> {
	value: string;
	align?: TooltipContentProps["align"];
}

export const CopyableValue: FC<CopyableValueProps> = ({
	value,
	align = "start",
	children,
	...attrs
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard({
		textToCopy: value,
	});
	const clickableProps = useClickable<HTMLSpanElement>(copyToClipboard);

	return (
		<MiniTooltip
			title={showCopiedSuccess ? "Copied!" : "Click to copy"}
			align={align}
		>
			<span {...attrs} {...clickableProps} css={{ cursor: "pointer" }}>
				{children}
			</span>
		</MiniTooltip>
	);
};
