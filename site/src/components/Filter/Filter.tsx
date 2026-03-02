import { useTheme } from "@emotion/react";
import Skeleton, { type SkeletonProps } from "@mui/material/Skeleton";
import type { Breakpoint } from "@mui/system/createTheme";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "api/errors";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { SearchField } from "components/SearchField/SearchField";
import { useDebouncedFunction } from "hooks/debounce";
import { ExternalLinkIcon, SlidersHorizontal } from "lucide-react";
import { type FC, type ReactNode, useEffect, useRef, useState } from "react";

type PresetFilter = {
	name: string;
	query: string;
};

type FilterValues = Record<string, string | undefined>;

type UseFilterConfig = {
	/**
	 * The fallback value to use in the event that no filter params can be
	 * parsed from the search params object.
	 */
	fallbackFilter?: string;
	searchParams: URLSearchParams;
	onSearchParamsChange: (newParams: URLSearchParams) => void;
	onUpdate?: (newValue: string) => void;
};

export type UseFilterResult = Readonly<{
	query: string;
	values: FilterValues;
	used: boolean;
	update: (newValues: string | FilterValues) => void;
	debounceUpdate: (newValues: string | FilterValues) => void;
	cancelDebounce: () => void;
}>;

export const useFilterParamsKey = "filter";

export const useFilter = ({
	fallbackFilter = "",
	searchParams,
	onSearchParamsChange,
	onUpdate,
}: UseFilterConfig): UseFilterResult => {
	const query = searchParams.get(useFilterParamsKey) ?? fallbackFilter;

	const update = (newValues: string | FilterValues) => {
		const serialized =
			typeof newValues === "string" ? newValues : stringifyFilter(newValues);
		const noUpdateNeeded = query === serialized;
		if (noUpdateNeeded) {
			return;
		}

		/**
		 * @todo 2025-07-15 - We have a slightly nasty bug here, where trying to
		 * update state via immutable state updates causes our code to break.
		 *
		 * In theory, it would be better to make a copy of the search params. We
		 * can then mutate and dispatch the copy instead of the original. Doing
		 * that causes other parts of our existing logic to break, though.
		 * That's a sign that our other code is slightly broken, and only just
		 * happens to work by chance right now.
		 */
		searchParams.set(useFilterParamsKey, serialized);
		onSearchParamsChange(searchParams);
		onUpdate?.(serialized);
	};

	const { debounced: debounceUpdate, cancelDebounce } = useDebouncedFunction(
		update,
		500,
	);

	return {
		query,
		update,
		debounceUpdate,
		cancelDebounce,
		values: parseFilterQuery(query),
		used: query !== "" && query !== fallbackFilter,
	};
};

export const parseFilter = (input: string): FilterValues => {
	const result: FilterValues = {};
	let i = 0;

	const skipWhitespace = () => {
		while (i < input.length && /\s/.test(input.charAt(i))) {
			i++;
		}
	};

	const parseKey = (): string => {
		const start = i;
		while (i < input.length && /[a-zA-Z0-9_-]/.test(input.charAt(i))) {
			i++;
		}
		return input.slice(start, i);
	};

	const parseQuoted = (): string => {
		i++; // skip opening "
		let value = "";

		while (i < input.length) {
			if (input.charAt(i) === "\\" && i + 1 < input.length) {
				value += input.charAt(i + 1);
				i += 2;
				continue;
			}

			if (input.charAt(i) === '"') {
				i++; // skip closing "
				break;
			}

			value += input.charAt(i);
			i++;
		}

		return value;
	};

	const parseUnquoted = (): string => {
		const start = i;
		while (
			i < input.length &&
			input.charAt(i) !== "," &&
			!/\s/.test(input.charAt(i))
		) {
			i++;
		}
		return input.slice(start, i);
	};

	const parseValue = (): string => {
		if (input.charAt(i) === '"') {
			return parseQuoted();
		}
		return parseUnquoted();
	};

	const parseValueList = (): string[] => {
		const values: string[] = [];

		while (i < input.length) {
			skipWhitespace();
			values.push(parseValue());
			skipWhitespace();

			if (input.charAt(i) === ",") {
				i++;
				continue;
			}

			break;
		}

		return values;
	};

	while (i < input.length) {
		skipWhitespace();
		if (i >= input.length) {
			break;
		}

		const key = parseKey();
		if (!key) {
			break;
		}
		skipWhitespace();

		if (input.charAt(i) !== ":") {
			break;
		}

		i++; // skip :
		skipWhitespace();
		const values = parseValueList();
		if (values.length === 1) {
			result[key] = values[0];
		}
		skipWhitespace();
	}

	return result;
};

const parseFilterQuery = (filterQuery: string): FilterValues => {
	return parseFilter(filterQuery);
};

const stringifyFilter = (filterValue: FilterValues): string => {
	let result = "";
	const formatValue = (value: string) => {
		const escaped = value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
		const requiresQuoting = /[\s,"]/.test(value);
		return requiresQuoting ? `"${escaped}"` : escaped;
	};

	for (const key in filterValue) {
		const value = filterValue[key];
		if (!value) {
			continue;
		}
		result += `${key}:${formatValue(value)} `;
	}

	return result.trim();
};

const BaseSkeleton: FC<SkeletonProps> = ({ children, ...skeletonProps }) => {
	return (
		<Skeleton
			variant="rectangular"
			height={36}
			{...skeletonProps}
			css={(theme) => ({
				backgroundColor: theme.palette.background.paper,
				borderRadius: "6px",
			})}
		>
			{children}
		</Skeleton>
	);
};

export const MenuSkeleton: FC = () => {
	return <BaseSkeleton css={{ minWidth: 200, flexShrink: 0 }} />;
};

type FilterProps = {
	filter: ReturnType<typeof useFilter>;
	optionsSkeleton: ReactNode;
	isLoading: boolean;
	learnMoreLink?: string;
	learnMoreLabel2?: string;
	learnMoreLink2?: string;
	error?: unknown;
	options?: ReactNode;
	presets: PresetFilter[];

	/**
	 * The CSS media query breakpoint that defines when the UI will try
	 * displaying all options on one row, regardless of the number of options
	 * present
	 */
	singleRowBreakpoint?: Breakpoint;
};

export const Filter: FC<FilterProps> = ({
	filter,
	isLoading,
	error,
	optionsSkeleton,
	options,
	learnMoreLink,
	learnMoreLabel2,
	learnMoreLink2,
	presets,
	singleRowBreakpoint = "lg",
}) => {
	const theme = useTheme();
	// Storing local copy of the filter query so that it can be updated more
	// aggressively without re-renders rippling out to the rest of the app every
	// single time. Exists for performance reasons - not really a good way to
	// remove this; render keys would cause the component to remount too often
	const [queryCopy, setQueryCopy] = useState(filter.query);
	const textboxInputRef = useRef<HTMLInputElement>(null);

	// Conditionally re-syncs the parent and local filter queries
	useEffect(() => {
		const hasSelfOrInnerFocus =
			textboxInputRef.current?.contains(document.activeElement) ?? false;

		// This doesn't address all state sync issues - namely, what happens if the
		// user removes focus just after this synchronizing effect fires. Also need
		// to rely on onBlur behavior as an extra safety measure
		if (!hasSelfOrInnerFocus) {
			setQueryCopy(filter.query);
		}
	}, [filter.query]);

	const shouldDisplayError = hasError(error) && isApiValidationError(error);

	return (
		<div
			css={{
				display: "flex",
				gap: 8,
				marginBottom: 16,
				flexWrap: "wrap",

				[theme.breakpoints.up(singleRowBreakpoint)]: {
					flexWrap: "nowrap",
				},
			}}
		>
			{isLoading ? (
				<>
					<BaseSkeleton width="100%" />
					{optionsSkeleton}
				</>
			) : (
				<>
					<PresetMenu
						value={filter.query}
						onSelect={(query) => filter.update(query)}
						presets={presets}
						learnMoreLink={learnMoreLink}
						learnMoreLabel2={learnMoreLabel2}
						learnMoreLink2={learnMoreLink2}
					/>
					<div className="flex flex-col gap-2 w-full">
						<SearchField
							ref={textboxInputRef}
							className="w-full"
							value={queryCopy}
							aria-label="Filter"
							aria-invalid={shouldDisplayError}
							onChange={(query) => {
								setQueryCopy(query);
								filter.debounceUpdate(query);
							}}
							onClear={() => {
								setQueryCopy("");
								filter.cancelDebounce();
								filter.update("");
							}}
							onBlur={() => {
								if (queryCopy === filter.query) return;
								setQueryCopy(filter.query);
							}}
							placeholder="Search..."
						/>
						{hasError(error) && (
							<span className="text-content-destructive text-sm">
								{getValidationErrorMessage(error)}
							</span>
						)}
					</div>
					{options}
				</>
			)}
		</div>
	);
};

interface PresetMenuProps {
	value: string;
	presets: PresetFilter[];
	learnMoreLink?: string;
	learnMoreLabel2?: string;
	learnMoreLink2?: string;
	onSelect: (query: string) => void;
}

const PresetMenu: FC<PresetMenuProps> = ({
	value,
	presets,
	learnMoreLink,
	learnMoreLabel2,
	learnMoreLink2,
	onSelect,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline">
					<SlidersHorizontal />
					Filters
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent side="bottom" align="start">
				<DropdownMenuRadioGroup value={value}>
					{presets.map((presetFilter) => (
						<DropdownMenuRadioItem
							value={presetFilter.query}
							onSelect={() => onSelect(presetFilter.query)}
							key={presetFilter.name}
						>
							{presetFilter.name}
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
				{(learnMoreLink || learnMoreLink2) && <DropdownMenuSeparator />}
				{learnMoreLink && (
					<DropdownMenuItem asChild>
						<a href={learnMoreLink} target="_blank">
							<ExternalLinkIcon className="size-icon-xs" />
							View advanced filtering
						</a>
					</DropdownMenuItem>
				)}
				{learnMoreLink2 && learnMoreLabel2 && (
					<DropdownMenuItem asChild>
						<a href={learnMoreLink2} target="_blank">
							<ExternalLinkIcon className="size-icon-xs" />
							{learnMoreLabel2}
						</a>
					</DropdownMenuItem>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
