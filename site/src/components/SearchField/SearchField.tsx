import { useTheme } from "@emotion/react";
import CloseIcon from "@mui/icons-material/CloseOutlined";
import SearchIcon from "@mui/icons-material/SearchOutlined";
import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import visuallyHidden from "@mui/utils/visuallyHidden";
import type { FC } from "react";

export type SearchFieldProps = Omit<TextFieldProps, "onChange"> & {
	onChange: (query: string) => void;
};

export const SearchField: FC<SearchFieldProps> = ({
	value = "",
	onChange,
	InputProps,
	...textFieldProps
}) => {
	const theme = useTheme();
	return (
		<TextField
			// Have to specify min width so that, as we keep adding more and
			// control options to the filter row, the text box doesn't have a
			// risk of shrinking so much that it becomes un-clickable
			css={{ minWidth: "320px" }}
			size="small"
			value={value}
			onChange={(e) => onChange(e.target.value)}
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
						<Tooltip title="Clear field">
							<IconButton
								size="small"
								onClick={() => {
									onChange("");
								}}
							>
								<CloseIcon css={{ fontSize: 14 }} />
								<span css={{ ...visuallyHidden }}>Clear field</span>
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
