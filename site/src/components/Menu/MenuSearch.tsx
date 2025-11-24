import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import type { FC } from "react";
export const MenuSearch: FC<SearchFieldProps> = (props) => {
	return (
		<SearchField
			fullWidth
			css={(theme) => ({
				"& fieldset": {
					border: 0,
					borderRadius: 0,
					// MUI has so many nested selectors that it's easier to just
					// override the border directly using the `!important` hack
					borderBottom: `1px solid ${theme.palette.divider} !important`,
				},
			})}
			{...props}
		/>
	);
};
