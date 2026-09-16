import type { FC } from "react";
import { columnColor } from "../../ChatBoard/boardLabels";

/** Small colored tag naming a chat's board column, used on sidebar rows. */
export const ColumnTag: FC<{ readonly name: string }> = ({ name }) => {
	const color = columnColor(name);
	return (
		<span
			className={`inline-flex h-4 shrink-0 items-center rounded px-[5px] text-[10.5px] font-medium ${color.tagBg} ${color.tagFg}`}
		>
			{name}
		</span>
	);
};
