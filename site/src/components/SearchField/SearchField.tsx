import {
	InputGroup,
	InputGroupAddon,
	InputGroupButton,
	InputGroupInput,
} from "components/InputGroup/InputGroup";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useEffectEvent } from "hooks/hookPolyfills";
import { SearchIcon, XIcon } from "lucide-react";
import { forwardRef, useLayoutEffect, useRef } from "react";

export type SearchFieldProps = {
	value: string;
	onChange: (query: string) => void;
	onClear?: () => void;
	placeholder?: string;
	className?: string;
	autoFocus?: boolean;
	"aria-label"?: string;
	"aria-invalid"?: boolean;
	onBlur?: () => void;
};

export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(
	(
		{
			onChange,
			onClear,
			value = "",
			placeholder = "Search...",
			className,
			autoFocus = false,
			onBlur,
			...ariaProps
		},
		forwardedRef,
	) => {
		const internalRef = useRef<HTMLInputElement | null>(null);
		const focusOnMount = useEffectEvent((): void => {
			if (autoFocus) {
				internalRef.current?.focus();
			}
		});
		useLayoutEffect(() => {
			focusOnMount();
		}, [focusOnMount]);

		const handleClear = () => {
			if (onClear) {
				onClear();
			} else {
				onChange("");
			}
		};

		const setRefs = (el: HTMLInputElement | null) => {
			internalRef.current = el;
			if (typeof forwardedRef === "function") {
				forwardedRef(el);
			} else if (forwardedRef) {
				forwardedRef.current = el;
			}
		};

		return (
			<InputGroup className={className}>
				<InputGroupAddon>
					<SearchIcon className="size-icon-sm" />
				</InputGroupAddon>
				<InputGroupInput
					ref={setRefs}
					className="flex-1 h-10"
					value={value}
					onChange={(e) => onChange(e.target.value)}
					onBlur={onBlur}
					placeholder={placeholder}
					{...ariaProps}
				/>
				{value !== "" && (
					<InputGroupAddon align="inline-end">
						<Tooltip>
							<TooltipTrigger asChild>
								<InputGroupButton onClick={handleClear} size="icon">
									<XIcon />
									<span className="sr-only">Clear search</span>
								</InputGroupButton>
							</TooltipTrigger>
							<TooltipContent align="end" sideOffset={8} alignOffset={-8}>
								Clear search
							</TooltipContent>
						</Tooltip>
					</InputGroupAddon>
				)}
			</InputGroup>
		);
	},
);
