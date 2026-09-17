import { cva } from "class-variance-authority";
import type { FC } from "react";
import { columnHue } from "./boardLabels";

const tag = cva(
	"inline-flex h-4 max-w-28 shrink-0 items-center truncate rounded px-[5px] text-[10.5px] font-medium text-content-primary",
	{
		variants: {
			hue: {
				neutral: "bg-surface-tertiary",
				purple: "bg-surface-purple",
				sky: "bg-surface-sky",
				green: "bg-surface-green",
				orange: "bg-surface-orange",
				magenta: "bg-surface-magenta",
				red: "bg-surface-red",
			},
		},
	},
);

interface ColumnTagProps {
	readonly name: string;
}

/** Small tinted tag naming a chat's board column, used on sidebar rows. */
export const ColumnTag: FC<ColumnTagProps> = ({ name }) => (
	<span className={tag({ hue: columnHue(name) })}>{name}</span>
);
