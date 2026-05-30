import { ArrowDownIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

export const ScrollToBottomButton: FC<{
	visible: boolean;
	onScrollToBottom: () => void;
}> = ({ visible, onScrollToBottom }) => {
	return (
		// Floating overlay above the scroll container.
		<div className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center py-2">
			<Button
				variant="outline"
				size="icon"
				className={cn(
					"rounded-full bg-surface-primary shadow-md transition-all duration-200",
					visible
						? "pointer-events-auto translate-y-0 opacity-100"
						: "translate-y-2 opacity-0",
				)}
				onClick={onScrollToBottom}
				aria-label="Scroll to bottom"
				aria-hidden={!visible || undefined}
				tabIndex={visible ? undefined : -1}
			>
				<ArrowDownIcon />
			</Button>
		</div>
	);
};
