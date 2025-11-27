import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useEffectEvent } from "hooks/hookPolyfills";
import { SearchIcon, XIcon } from "lucide-react";
import { type FC, useLayoutEffect, useRef } from "react";

export type SearchFieldProps = Omit<TextFieldProps, "onChange"> & {
	onChange: (query: string) => void;
	autoFocus?: boolean;
};

export const SearchField: FC<SearchFieldProps> = ({
	InputProps,
	onChange,
	value = "",
	autoFocus = false,
	...textFieldProps
}) => {
	// MUI's autoFocus behavior is wonky. If you set autoFocus=true, the
	// component will keep getting focus on every single render, even if there
	// are other input elements on screen. We want this to be one-time logic
	const inputRef = useRef<HTMLInputElement | null>(null);
	const focusOnMount = useEffectEvent((): void => {
		if (autoFocus) {
			inputRef.current?.focus();
		}
	});
	useLayoutEffect(() => {
		focusOnMount();
	}, [focusOnMount]);

	return (
		<TextField
			inputRef={inputRef}
			// Specifying min width so that the text box can't shrink so much
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
						<Tooltip>
							<TooltipTrigger asChild>
								<IconButton
									size="small"
									onClick={() => {
										onChange("");
									}}
								>
									<XIcon className="size-icon-xs" />
									<span className="sr-only">Clear search</span>
								</IconButton>
							</TooltipTrigger>
							<TooltipContent side="bottom">Clear search</TooltipContent>
						</Tooltip>
					</InputAdornment>
				),
				...InputProps,
			}}
			{...textFieldProps}
		/>
	);
};
