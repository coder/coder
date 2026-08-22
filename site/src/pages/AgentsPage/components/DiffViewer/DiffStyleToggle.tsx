import { ColumnsIcon, RowsIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";
import type { DiffStyle } from "./DiffViewer";

interface DiffStyleToggleProps {
	value: DiffStyle;
	onChange: (style: DiffStyle) => void;
}

export const DiffStyleToggle: FC<DiffStyleToggleProps> = ({
	value,
	onChange,
}) => {
	return (
		<div className="flex h-6 items-stretch overflow-hidden rounded-md border border-solid border-border-default">
			<button
				type="button"
				onClick={() => onChange("unified")}
				aria-label="Unified diff"
				aria-pressed={value === "unified"}
				className={cn(
					"flex cursor-pointer items-center border-none px-1.5 transition-colors",
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
				className={cn(
					"flex cursor-pointer items-center border-0 border-l border-solid border-border-default px-1.5 transition-colors",
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
