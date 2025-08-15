import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import { SearchIcon, XIcon } from "lucide-react";
import type { FC } from "react";

export type SearchFieldProps = Omit<TextFieldProps, "onChange"> & {
	onChange: (query: string) => void;
	autoFocus?: boolean;
};

export const SearchField: FC<SearchFieldProps> = ({
	value = "",
	onChange,
	autoFocus = false,
	InputProps,
	...textFieldProps
}) => {
	return (
		<TextField
			autoFocus={autoFocus}
			// Specifying `minWidth` so that the text box can't shrink so much
			// that it becomes un-clickable as we add more filter controls
			className="min-w-[280px]"
			size="small"
			value={value}
			onChange={(e) => onChange(e.target.value)}
			InputProps={{
				startAdornment: (
					<InputAdornment position="start">
						<SearchIcon className="size-icon-xs text-content-secondary" />
					</InputAdornment>
				),
				endAdornment: value !== "" && (
					<InputAdornment position="end">
						<Tooltip title="Clear search">
							<IconButton size="small" onClick={() => onChange("")}>
								<XIcon className="size-icon-xs" />
								<span className="sr-only">Clear search</span>
							</IconButton>
						</Tooltip>
					</InputAdornment>
				),
				...InputProps,
			}}
			{...textFieldProps}
		/>
	);
};
