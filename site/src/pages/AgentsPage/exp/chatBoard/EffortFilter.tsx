import { cva } from "class-variance-authority";
import type { FC } from "react";
import type { EffortCount } from "./boardApi";

const pill = cva(
	"flex h-6 items-center gap-1 rounded-full border px-2.5 text-[11px] leading-none",
	{
		variants: {
			selected: {
				true: "border-content-link bg-content-link/10 text-content-primary",
				false:
					"border-border bg-transparent text-content-secondary hover:border-content-secondary hover:text-content-primary",
			},
		},
	},
);

interface EffortFilterProps {
	/** Every effort on the board with its card count; the row is hidden when empty. */
	readonly efforts: readonly EffortCount[];
	readonly cardCount: number;
	/** The selected effort; null shows every card. */
	readonly value: string | null;
	readonly onChange: (effort: string | null) => void;
}

/** One pill per effort under the header, plus All; a radio group since one is always selected. */
export const EffortFilter: FC<EffortFilterProps> = ({
	efforts,
	cardCount,
	value,
	onChange,
}) => {
	if (efforts.length === 0) return null;
	return (
		<div
			role="radiogroup"
			aria-label="Filter by effort"
			className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border px-5 py-2"
		>
			{[{ name: null, count: cardCount }, ...efforts].map(({ name, count }) => (
				<button
					key={name ?? ""}
					type="button"
					role="radio"
					aria-checked={value === name}
					className={pill({ selected: value === name })}
					onClick={() => onChange(name)}
				>
					{name ?? "All"}
					<span className="tabular-nums text-content-secondary/70">
						{count}
					</span>
				</button>
			))}
		</div>
	);
};
