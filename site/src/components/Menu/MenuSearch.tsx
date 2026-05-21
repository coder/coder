import type { FC } from "react";
import {
	SearchField,
	type SearchFieldProps,
} from "#/components/SearchField/SearchField";
export const MenuSearch: FC<SearchFieldProps> = (props) => {
	return <SearchField className="w-full [&_input]:rounded-none" {...props} />;
};
