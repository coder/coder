import emojiData, {
	type SheetPosition,
	type SkinVariation,
} from "virtual:emoji-manifest";
import {
	AppleIcon,
	CircleEllipsisIcon,
	CodeIcon,
	FlagIcon,
	HandIcon,
	LightbulbIcon,
	type LucideIcon,
	PawPrintIcon,
	PlaneIcon,
	ShapesIcon,
	SmileIcon,
	TrophyIcon,
} from "lucide-react";
import {
	type CSSProperties,
	type FC,
	type KeyboardEvent,
	type RefObject,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { preload } from "react-dom";
import {
	Command,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import icons from "#/theme/icons.json";
import { cn } from "#/utils/cn";

const ICON_CATEGORY = "Coder icons";
const DEFAULT_TONE = "default";

const CATEGORY_ICONS = new Map<string, LucideIcon>([
	["Smileys & Emotion", SmileIcon],
	["People & Body", HandIcon],
	["Animals & Nature", PawPrintIcon],
	["Food & Drink", AppleIcon],
	["Travel & Places", PlaneIcon],
	["Activities", TrophyIcon],
	["Objects", LightbulbIcon],
	["Symbols", ShapesIcon],
	["Flags", FlagIcon],
	[ICON_CATEGORY, CodeIcon],
]);
const SPRITESHEET_URL = `/emojis/${emojiData.sheet.file}?v=${emojiData.sheet.hash}`;
const SPRITESHEET_SIZE = `${emojiData.sheet.columns * 100}% ${emojiData.sheet.rows * 100}%`;

preload(SPRITESHEET_URL, { as: "image", fetchPriority: "low" });

/**
 * The manifest holds nearly 2,000 emoji. Browsing is scoped to one category and
 * search renders only the highest ranked results, so a one-character query
 * cannot mount every image.
 */
const SEARCH_RESULT_LIMIT = 64;

/**
 * Vertical navigation jumps a whole row, so the rendered column count and the
 * navigation step must agree. The grid reads this same constant through the
 * `--emoji-picker-columns` custom property.
 */
const GRID_COLUMNS = 8;

type GridStyle = CSSProperties & Record<"--emoji-picker-columns", number>;

const gridStyle: GridStyle = { "--emoji-picker-columns": GRID_COLUMNS };

const GRID_STEPS = new Map<string, number>([
	["ArrowLeft", -1],
	["ArrowRight", 1],
	["ArrowUp", -GRID_COLUMNS],
	["ArrowDown", GRID_COLUMNS],
]);

const SKIN_TONES = [
	{ value: DEFAULT_TONE, label: "Default" },
	{ value: "1f3fb", label: "Light" },
	{ value: "1f3fc", label: "Medium light" },
	{ value: "1f3fd", label: "Medium" },
	{ value: "1f3fe", label: "Medium dark" },
	{ value: "1f3ff", label: "Dark" },
];

const spriteStyle = ({ sheetX, sheetY }: SheetPosition): CSSProperties => ({
	backgroundImage: `url("${SPRITESHEET_URL}")`,
	backgroundPosition: `${(sheetX / (emojiData.sheet.columns - 1)) * 100}% ${(sheetY / (emojiData.sheet.rows - 1)) * 100}%`,
	backgroundSize: SPRITESHEET_SIZE,
});

const Sprite: FC<{ position: SheetPosition; className: string }> = ({
	position,
	className,
}) => (
	<span
		aria-hidden="true"
		data-testid="emoji-sprite"
		className={`shrink-0 bg-no-repeat ${className}`}
		style={spriteStyle(position)}
	/>
);

/** Waving hand ships every single tone, so it doubles as the tone swatch. */
const wavingHand = emojiData.emojis.find((emoji) => emoji.id === "wave");
const toneSwatch = (tone: string): SheetPosition => {
	if (!wavingHand) {
		throw new Error("Waving hand is missing from the emoji manifest");
	}
	if (tone === DEFAULT_TONE) {
		return wavingHand;
	}
	const variation = wavingHand.skins?.find((skin) => skin.tone === tone);
	if (!variation) {
		throw new Error(`Waving hand is missing the ${tone} skin variation`);
	}
	return variation;
};

/**
 * Manifest names are canonical and lowercase apart from acronyms and proper
 * nouns, so only the first character is touched. Lowercasing the rest would
 * turn "ATM sign" into "Atm sign".
 */
const displayName = (name: string) =>
	name.charAt(0).toUpperCase() + name.slice(1);

/** Ids, keywords and icon file names separate words with hyphens or underscores. */
const normalize = (value: string) =>
	value.toLowerCase().replace(/[-_]+/g, " ").replace(/\s+/g, " ").trim();

const words = (value: string) => normalize(value).split(" ").filter(Boolean);

const RANK_EXACT = 0;
const RANK_PREFIX = 1;
const RANK_KEYWORD = 2;
const RANK_SUBSTRING = 3;
const RANK_NONE = -1;

type SearchIndex = {
	id: string;
	name: string;
	aliases: string[];
	keywords: string[];
	/** Everything above, joined, for order-independent term matching. */
	haystack: string;
};

type PickerOption = {
	/** cmdk value and React key; unique across emoji and icons. */
	id: string;
	label: string;
	category: string;
	/** Selection URL when no skin tone applies. */
	src: string;
	sheet?: SheetPosition;
	skins?: SkinVariation[];
	search: SearchIndex;
};

const buildSearchIndex = (
	id: string,
	name: string,
	aliases: string[],
	keywords: string[],
): SearchIndex => {
	const unique = [...new Set(keywords.filter(Boolean))];
	return {
		id,
		name,
		aliases,
		keywords: unique,
		haystack: [id, name, ...aliases, ...unique].join(" "),
	};
};

const emojiOptions: PickerOption[] = emojiData.emojis.map((emoji) => ({
	id: emoji.unified,
	label: displayName(emoji.name),
	category: emoji.category,
	src: `/emojis/${emoji.image}`,
	sheet: emoji,
	...(emoji.skins && { skins: emoji.skins }),
	search: buildSearchIndex(
		normalize(emoji.id),
		normalize(emoji.name),
		(emoji.aliases ?? []).map(normalize),
		[
			...emoji.keywords.flatMap(words),
			// Text aliases such as ":D" are punctuation, so splitting them into
			// words would leave nothing to match.
			...(emoji.textAliases ?? []).map((alias) => alias.toLowerCase()),
			...words(emoji.subcategory),
		],
	),
}));

const iconOptions: PickerOption[] = icons
	.filter((icon) => !DEPRECATED_ICONS.includes(icon))
	.map((icon) => {
		const name = icon.replace(/\.[^.]+$/, "");
		const normalized = normalize(name);
		return {
			id: icon,
			label: name,
			category: ICON_CATEGORY,
			src: `/icon/${icon}`,
			search: buildSearchIndex(
				normalized,
				normalized,
				[icon.toLowerCase()],
				[...words(ICON_CATEGORY), ...words(name)],
			),
		};
	});

const allOptions = [...emojiOptions, ...iconOptions];

const optionsByCategory = new Map<string, PickerOption[]>();
for (const option of allOptions) {
	const group = optionsByCategory.get(option.category);
	if (group) {
		group.push(option);
	} else {
		optionsByCategory.set(option.category, [option]);
	}
}
const categories = [...emojiData.categories, ICON_CATEGORY].map((label) => ({
	label,
	icon: CATEGORY_ICONS.get(label) ?? CircleEllipsisIcon,
}));
const DEFAULT_CATEGORY = categories[0].label;

/**
 * Every query term has to match somewhere, in any order, and the tier decides
 * the order results appear in. Ranking is what keeps short queries usable: with
 * plain substring matching, "code" buried the Coder icons behind every emoji
 * whose keywords happen to contain those letters.
 */
const rankOption = (option: PickerOption, query: string, terms: string[]) => {
	const { search } = option;
	if (!terms.every((term) => search.haystack.includes(term))) {
		return RANK_NONE;
	}
	if (
		search.id === query ||
		search.name === query ||
		search.aliases.includes(query)
	) {
		return RANK_EXACT;
	}
	if (
		search.id.startsWith(query) ||
		search.name.startsWith(query) ||
		search.aliases.some((alias) => alias.startsWith(query))
	) {
		return RANK_PREFIX;
	}
	if (search.keywords.some((keyword) => keyword.startsWith(query))) {
		return RANK_KEYWORD;
	}
	return RANK_SUBSTRING;
};

const searchOptions = (query: string): PickerOption[] => {
	const terms = query.split(" ").filter(Boolean);
	return allOptions
		.map((option, index) => ({
			option,
			index,
			rank: rankOption(option, query, terms),
		}))
		.filter((entry) => entry.rank !== RANK_NONE)
		.sort((a, b) => a.rank - b.rank || a.index - b.index)
		.map((entry) => entry.option);
};

/**
 * Resolve a tone to its exact single-tone variation, falling back to the
 * same-tone composite that multi-person emoji use instead. Mixed-tone
 * composites are not offered, so anything else keeps the toneless image.
 */
const resolveSkin = (
	option: PickerOption,
	tone: string,
): SkinVariation | undefined => {
	if (tone === DEFAULT_TONE || !option.skins) {
		return undefined;
	}
	return (
		option.skins.find((skin) => skin.tone === tone) ??
		option.skins.find((skin) => skin.tone === `${tone}-${tone}`)
	);
};

const resolveSrc = (option: PickerOption, tone: string): string => {
	const variation = resolveSkin(option, tone);
	return variation ? `/emojis/${variation.image}` : option.src;
};

const resolveSheet = (
	option: PickerOption,
	tone: string,
): SheetPosition | undefined => resolveSkin(option, tone) ?? option.sheet;

type CategoryNavigationProps = {
	currentCategory: string;
	inputRef: RefObject<HTMLInputElement | null>;
	searching: boolean;
	onSelect: (category: string) => void;
};

const CategoryNavigation: FC<CategoryNavigationProps> = ({
	currentCategory,
	inputRef,
	searching,
	onSelect,
}) => {
	const [focusedCategory, setFocusedCategory] = useState(currentCategory);
	const handleKeys = (event: KeyboardEvent<HTMLDivElement>) => {
		if (!(event.target instanceof HTMLButtonElement)) {
			return;
		}
		const buttons = Array.from(
			event.currentTarget.querySelectorAll<HTMLButtonElement>(
				"[data-emoji-category]",
			),
		);
		const current = buttons.indexOf(event.target);
		if (current === -1) {
			return;
		}
		let target: number | undefined;
		switch (event.key) {
			case "ArrowLeft":
				target = (current + buttons.length - 1) % buttons.length;
				break;
			case "ArrowRight":
				target = (current + 1) % buttons.length;
				break;
			case "Home":
				target = 0;
				break;
			case "End":
				target = buttons.length - 1;
				break;
		}
		if (target === undefined) {
			return;
		}
		event.preventDefault();
		buttons[target]?.focus();
	};

	return (
		<div
			role="toolbar"
			aria-label="Emoji and icon categories"
			onKeyDown={handleKeys}
			className="flex w-full border-0 border-b border-border border-solid"
		>
			{categories.map(({ label, icon: CategoryIcon }) => {
				const selected = !searching && currentCategory === label;
				return (
					<button
						key={label}
						type="button"
						data-emoji-category
						aria-current={selected ? "true" : undefined}
						aria-label={label}
						title={label}
						tabIndex={focusedCategory === label ? 0 : -1}
						onMouseDown={(event) => {
							// Keep mouse category selection from moving focus out of search.
							if (document.activeElement === inputRef.current) {
								event.preventDefault();
							}
						}}
						onFocus={() => setFocusedCategory(label)}
						onClick={() => {
							setFocusedCategory(label);
							onSelect(label);
						}}
						className={cn(
							"-mb-px flex min-w-0 flex-1 cursor-pointer justify-center border-0 border-b border-solid border-b-transparent bg-transparent py-2 text-content-secondary transition-colors",
							"hover:text-content-primary focus-visible:z-10 focus-visible:rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-inset",
							selected && "border-b-content-primary text-content-primary",
						)}
					>
						<CategoryIcon aria-hidden="true" className="size-4" />
					</button>
				);
			})}
		</div>
	);
};

type EmojiPickerProps = {
	/**
	 * Receives the selection URL, either `/emojis/<image>.png` for an emoji or
	 * `/icon/<file>` for a Coder icon.
	 */
	onSelect: (url: string) => void;
};

const EmojiPicker: FC<EmojiPickerProps> = ({ onSelect }) => {
	const [category, setCategory] = useState(DEFAULT_CATEGORY);
	const [tone, setTone] = useState(DEFAULT_TONE);
	const [search, setSearch] = useState("");
	const [selectedId, setSelectedId] = useState("");
	const inputRef = useRef<HTMLInputElement>(null);
	const listRef = useRef<HTMLDivElement>(null);
	const itemNodes = useRef(new Map<string, HTMLElement>());

	const query = normalize(search);
	const matches = useMemo(
		() =>
			query ? searchOptions(query) : (optionsByCategory.get(category) ?? []),
		[query, category],
	);
	const visible = query ? matches.slice(0, SEARCH_RESULT_LIMIT) : matches;
	const isCapped = matches.length > visible.length;
	const activeId = visible.some((option) => option.id === selectedId)
		? selectedId
		: (visible[0]?.id ?? "");
	let statusMessage = "";
	let statusClassName = "sr-only";
	if (isCapped) {
		statusMessage = `Showing the first ${SEARCH_RESULT_LIMIT} matches. Keep typing to narrow the search.`;
		statusClassName = "m-0 px-3 pt-2 text-content-secondary text-xs";
	} else if (visible.length === 0) {
		statusMessage = query
			? "No emojis or icons match your search."
			: `No emojis or icons in ${category}.`;
		statusClassName =
			"m-0 px-3 py-6 text-center text-content-secondary text-sm";
	} else if (query) {
		statusMessage = "Showing emoji and icon search results.";
	} else {
		statusMessage = `Browsing ${category}.`;
	}
	const toneLabel =
		SKIN_TONES.find((skinTone) => skinTone.value === tone)?.label ?? "Default";
	const resetListScroll = () => {
		if (listRef.current) {
			listRef.current.scrollTop = 0;
		}
	};
	const updateSearch = (nextSearch: string) => {
		resetListScroll();
		setSearch(nextSearch);
	};
	const selectCategory = (nextCategory: string) => {
		resetListScroll();
		setCategory(nextCategory);
		setSearch("");
	};

	// cmdk does not update aria-activedescendant when its value is controlled.
	// Synchronize it after cmdk has applied its own input state.
	useEffect(() => {
		const activeDescendant = itemNodes.current.get(activeId)?.id;
		if (activeDescendant) {
			inputRef.current?.setAttribute("aria-activedescendant", activeDescendant);
		} else {
			inputRef.current?.removeAttribute("aria-activedescendant");
		}
	});

	const moveSelection = (step: number) => {
		if (visible.length === 0) {
			return;
		}
		const current = Math.max(
			visible.findIndex((option) => option.id === activeId),
			0,
		);
		const target = Math.min(Math.max(current + step, 0), visible.length - 1);
		const option = visible[target];
		setSelectedId(option.id);
		itemNodes.current.get(option.id)?.scrollIntoView({ block: "nearest" });
	};

	// cmdk walks the list one item at a time, which reads as random movement in a
	// grid. Handling the arrows here and calling preventDefault stops cmdk from
	// applying its linear step on top of the row jump.
	const handleGridKeys = (event: KeyboardEvent<HTMLDivElement>) => {
		if (
			event.target instanceof HTMLInputElement &&
			(event.key === "Home" || event.key === "End") &&
			event.target.value
		) {
			event.preventDefault();
			const { selectionDirection, selectionEnd, selectionStart, value } =
				event.target;
			const position = event.key === "Home" ? 0 : value.length;
			if (event.shiftKey) {
				const anchor =
					selectionDirection === "backward" ? selectionEnd : selectionStart;
				event.target.setSelectionRange(
					Math.min(anchor ?? 0, position),
					Math.max(anchor ?? 0, position),
					event.key === "Home" ? "backward" : "forward",
				);
			} else {
				event.target.setSelectionRange(position, position);
			}
			return;
		}

		const step = GRID_STEPS.get(event.key);
		if (step === undefined) {
			return;
		}

		if (
			event.target instanceof HTMLInputElement &&
			(event.key === "ArrowLeft" || event.key === "ArrowRight")
		) {
			const { selectionStart, selectionEnd, value } = event.target;
			const hasModifier =
				event.altKey || event.ctrlKey || event.metaKey || event.shiftKey;
			const hasSelection = selectionStart !== selectionEnd;
			const isAtBoundary =
				event.key === "ArrowLeft"
					? selectionStart === 0
					: selectionEnd === value.length;
			if (hasModifier || hasSelection || !isAtBoundary) {
				return;
			}
		}

		event.preventDefault();
		moveSelection(step);
	};

	return (
		<div className="flex max-h-[var(--radix-popper-available-height)] w-80 flex-col overflow-hidden">
			<CategoryNavigation
				currentCategory={category}
				inputRef={inputRef}
				searching={Boolean(query)}
				onSelect={selectCategory}
			/>

			<Command
				className="min-h-0"
				shouldFilter={false}
				vimBindings={false}
				label="Search emojis and icons"
				value={activeId}
				onValueChange={setSelectedId}
				onKeyDown={handleGridKeys}
			>
				<CommandInput
					ref={inputRef}
					autoFocus
					value={search}
					onValueChange={updateSearch}
					placeholder="Search emojis and icons"
				/>

				<p role="status" className={statusClassName}>
					{statusMessage}
				</p>

				<CommandList
					ref={listRef}
					label={
						query
							? "Emoji and icon search results"
							: `${category} emojis and icons`
					}
					style={gridStyle}
					className="min-h-0 [&_[cmdk-list-sizer]]:grid [&_[cmdk-list-sizer]]:grid-cols-[repeat(var(--emoji-picker-columns),minmax(0,1fr))] [&_[cmdk-list-sizer]]:gap-1 [&_[cmdk-list-sizer]]:p-2"
				>
					{visible.map((option) => {
						const src = resolveSrc(option, tone);
						const sheet = resolveSheet(option, tone);
						return (
							<CommandItem
								key={option.id}
								ref={(node) => {
									if (node) {
										itemNodes.current.set(option.id, node);
									} else {
										itemNodes.current.delete(option.id);
									}
								}}
								value={option.id}
								aria-label={option.label}
								onSelect={() => onSelect(src)}
								className="aspect-square cursor-pointer justify-center p-1 data-[selected=true]:ring-2 data-[selected=true]:ring-content-link data-[selected=true]:ring-inset"
							>
								{sheet ? (
									<Sprite position={sheet} className="size-full" />
								) : (
									<ExternalImage
										alt=""
										src={src}
										loading="lazy"
										className="max-h-full max-w-full object-contain"
									/>
								)}
							</CommandItem>
						);
					})}
				</CommandList>
			</Command>

			<div className="flex justify-end border-0 border-border border-t border-solid p-2">
				<Select value={tone} onValueChange={setTone}>
					<SelectTrigger
						aria-label={`Skin tone: ${toneLabel}`}
						className="h-8 w-32 shrink-0 text-xs"
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{SKIN_TONES.map((skinTone) => (
							<SelectItem key={skinTone.value} value={skinTone.value}>
								<span className="flex items-center gap-2">
									<Sprite
										position={toneSwatch(skinTone.value)}
										className="size-4"
									/>
									{skinTone.label}
								</span>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
		</div>
	);
};

export default EmojiPicker;
