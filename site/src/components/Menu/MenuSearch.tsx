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
				"[&_fieldset]:!border-0 [&_fieldset]:!border-b",
				"[&_fieldset]:!border-surface-quaternary",
			)}
			{...props}
		/>
	);
};
