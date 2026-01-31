import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import type { FC } from "react";
export const MenuSearch: FC<SearchFieldProps> = (props) => {
	return <SearchField className="w-full [&_input]:rounded-none" {...props} />;
};
