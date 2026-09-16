import type { FC } from "react";
import { columnColor } from "./boardLabels";

/** Small tinted tag naming a chat's board column, used on sidebar rows. */
export const ColumnTag: FC<{ readonly name: string }> = ({ name }) => (
	<span
		className={`inline-flex h-4 max-w-28 shrink-0 items-center truncate rounded px-[5px] text-[10.5px] font-medium text-content-primary ${columnColor(name).tagBg}`}
	>
		{name}
	</span>
);
