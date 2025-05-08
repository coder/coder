import { useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import visuallyHidden from "@mui/utils/visuallyHidden";
import { X as CloseIcon, Search as SearchIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";

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
	const theme = useTheme();
	const inputRef = useRef<HTMLInputElement>(null);

	if (autoFocus) {
		useEffect(() => {
			inputRef.current?.focus();
		});
	}
	return (
		<TextField
			// Specifying `minWidth` so that the text box can't shrink so much
			// that it becomes un-clickable as we add more filter controls
			css={{ minWidth: "280px" }}
			size="small"
			value={value}
			onChange={(e) => onChange(e.target.value)}
			inputRef={inputRef}
			InputProps={{
				startAdornment: (
					<InputAdornment position="start">
						<SearchIcon
							css={{
								fontSize: 16,
								color: theme.palette.text.secondary,
							}}
						/>
					</InputAdornment>
				),
				endAdornment: value !== "" && (
					<InputAdornment position="end">
						<Tooltip title="Clear search">
							<IconButton
								size="small"
								onClick={() => {
									onChange("");
								}}
							>
								<CloseIcon css={{ fontSize: 14 }} />
								<span css={{ ...visuallyHidden }}>Clear search</span>
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
