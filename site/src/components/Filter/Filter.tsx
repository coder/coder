import { useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Skeleton, { type SkeletonProps } from "@mui/material/Skeleton";
import type { Breakpoint } from "@mui/system/createTheme";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "api/errors";
import { InputGroup } from "components/InputGroup/InputGroup";
import { SearchField } from "components/SearchField/SearchField";
import { useDebouncedFunction } from "hooks/debounce";
import {
	ChevronDown as KeyboardArrowDown,
	ExternalLink as OpenInNewOutlined,
} from "lucide-react";
import { type FC, type ReactNode, useEffect, useRef, useState } from "react";
import type { useSearchParams } from "react-router-dom";

export type PresetFilter = {
	name: string;
	query: string;
};

type FilterValues = Record<string, string | undefined>;

type UseFilterConfig = {
	/**
	 * The fallback value to use in the event that no filter params can be parsed
	 * from the search params object. This value is allowed to change on
	 * re-renders.
	 */
	fallbackFilter?: string;
	searchParamsResult: ReturnType<typeof useSearchParams>;
	onUpdate?: (newValue: string) => void;
};

export const useFilterParamsKey = "filter";

export const useFilter = ({
	fallbackFilter = "",
	searchParamsResult,
	onUpdate,
}: UseFilterConfig) => {
	const [searchParams, setSearchParams] = searchParamsResult;
	const query = searchParams.get(useFilterParamsKey) ?? fallbackFilter;

	const update = (newValues: string | FilterValues) => {
		const serialized =
			typeof newValues === "string" ? newValues : stringifyFilter(newValues);

		searchParams.set(useFilterParamsKey, serialized);
		setSearchParams(searchParams);

		if (onUpdate !== undefined) {
			onUpdate(serialized);
		}
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

export type UseFilterResult = ReturnType<typeof useFilter>;

const parseFilterQuery = (filterQuery: string): FilterValues => {
	if (filterQuery === "") {
		return {};
	}

	const pairs = filterQuery.split(" ");
	const result: FilterValues = {};

	for (const pair of pairs) {
		const [key, value] = pair.split(":") as [
			keyof FilterValues,
			string | undefined,
		];
		if (value) {
			result[key] = value;
		}
	}

	return result;
};

const stringifyFilter = (filterValue: FilterValues): string => {
	let result = "";

	for (const key in filterValue) {
		const value = filterValue[key];
		if (value) {
			result += `${key}:${value} `;
		}
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
					<InputGroup css={{ width: "100%" }}>
						<PresetMenu
							onSelect={(query) => filter.update(query)}
							presets={presets}
							learnMoreLink={learnMoreLink}
							learnMoreLabel2={learnMoreLabel2}
							learnMoreLink2={learnMoreLink2}
						/>
						<SearchField
							css={{ flex: 1 }}
							error={shouldDisplayError}
							helperText={
								shouldDisplayError
									? getValidationErrorMessage(error)
									: undefined
							}
							placeholder="Search..."
							value={queryCopy}
							onChange={(query) => {
								setQueryCopy(query);
								filter.debounceUpdate(query);
							}}
							InputProps={{
								ref: textboxInputRef,
								"aria-label": "Filter",
								onBlur: () => {
									if (queryCopy !== filter.query) {
										setQueryCopy(filter.query);
									}
								},
							}}
						/>
					</InputGroup>
					{options}
				</>
			)}
		</div>
	);
};

interface PresetMenuProps {
	presets: PresetFilter[];
	learnMoreLink?: string;
	learnMoreLabel2?: string;
	learnMoreLink2?: string;
	onSelect: (query: string) => void;
}

const PresetMenu: FC<PresetMenuProps> = ({
	presets,
	learnMoreLink,
	learnMoreLabel2,
	learnMoreLink2,
	onSelect,
}) => {
	const [isOpen, setIsOpen] = useState(false);
	const anchorRef = useRef<HTMLButtonElement>(null);
	const theme = useTheme();

	return (
		<>
			<Button
				onClick={() => setIsOpen(true)}
				ref={anchorRef}
				endIcon={<KeyboardArrowDown />}
			>
				Filters
			</Button>
			<Menu
				id="filter-menu"
				anchorEl={anchorRef.current}
				open={isOpen}
				onClose={() => setIsOpen(false)}
				anchorOrigin={{
					vertical: "bottom",
					horizontal: "left",
				}}
				transformOrigin={{
					vertical: "top",
					horizontal: "left",
				}}
				css={{ "& .MuiMenu-paper": { paddingTop: 8, paddingBottom: 8 } }}
			>
				{presets.map((presetFilter) => (
					<MenuItem
						css={{ fontSize: 14 }}
						key={presetFilter.name}
						onClick={() => {
							onSelect(presetFilter.query);
							setIsOpen(false);
						}}
					>
						{presetFilter.name}
					</MenuItem>
				))}
				{learnMoreLink && (
					<Divider css={{ borderColor: theme.palette.divider }} />
				)}
				{learnMoreLink && (
					<MenuItem
						component="a"
						href={learnMoreLink}
						target="_blank"
						css={{ fontSize: 13, fontWeight: 500 }}
						onClick={() => {
							setIsOpen(false);
						}}
					>
						<OpenInNewOutlined css={{ fontSize: "14px !important" }} />
						View advanced filtering
					</MenuItem>
				)}
				{learnMoreLink2 && learnMoreLabel2 && (
					<MenuItem
						component="a"
						href={learnMoreLink2}
						target="_blank"
						css={{ fontSize: 13, fontWeight: 500 }}
						onClick={() => {
							setIsOpen(false);
						}}
					>
						<OpenInNewOutlined css={{ fontSize: "14px !important" }} />
						{learnMoreLabel2}
					</MenuItem>
				)}
			</Menu>
		</>
	);
};
