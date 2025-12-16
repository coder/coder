import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import type { FC } from "react";
import { cn } from "utils/cn";

export const MenuSearch: FC<SearchFieldProps> = (props) => {
	return (
		<SearchField
			fullWidth
			className={cn(
				"[&_fieldset]:border-0 [&_fieldset]:rounded-none",
				"[&_fieldset]:!border-surface-quaternary",
				"[&_fieldset]:!border-0",
				// MUI has so many nested selectors that it's easier to just
				// override the border directly using the `!important` hack
				"[&_fieldset]:!border-b",
			)}
			{...props}
		/>
	);
};
