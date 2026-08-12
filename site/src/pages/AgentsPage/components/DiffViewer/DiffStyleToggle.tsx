import { ColumnsIcon, RowsIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";
import type { DiffStyle } from "./DiffViewer";

interface DiffStyleToggleProps {
	value: DiffStyle;
	onChange: (style: DiffStyle) => void;
	disabled?: boolean;
	disabledTitle?: string;
}

/**
 * Segmented control that toggles between unified and split diff layouts.
 * Rendered inside the Git panel sub-header on both the remote and local
 * views so the control follows the diff it configures.
 */
export const DiffStyleToggle: FC<DiffStyleToggleProps> = ({
	value,
	onChange,
	disabled,
	disabledTitle,
}) => {
	return (
		<div className="flex h-6 items-stretch overflow-hidden rounded-md border border-solid border-border-default">
			<button
				type="button"
				onClick={() => onChange("unified")}
				aria-label="Unified diff"
				aria-pressed={value === "unified"}
				disabled={disabled}
				title={disabled ? disabledTitle : undefined}
				className={cn(
					"flex cursor-pointer items-center border-none px-1.5 transition-colors disabled:cursor-default disabled:opacity-50",
					value === "unified"
						? "bg-surface-quaternary/25 text-content-primary"
						: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
				)}
			>
				<RowsIcon className="size-3.5" />
			</button>
			<button
				type="button"
				onClick={() => onChange("split")}
				aria-label="Split diff"
				aria-pressed={value === "split"}
				disabled={disabled}
				title={disabled ? disabledTitle : undefined}
				className={cn(
					"flex cursor-pointer items-center border-0 border-l border-solid border-border-default px-1.5 transition-colors disabled:cursor-default disabled:opacity-50",
					value === "split"
						? "bg-surface-quaternary/25 text-content-primary"
						: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
				)}
			>
				<ColumnsIcon className="size-3.5" />
			</button>
		</div>
	);
};
