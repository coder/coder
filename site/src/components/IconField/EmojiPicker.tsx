import { cn } from "cn";
import {
	type EmojiPickerListCategoryHeaderProps,
	type EmojiPickerListEmojiProps,
	type EmojiPickerListRowProps,
	EmojiPicker as Frimousse,
	useSkinTone,
} from "frimousse";
import {
	AppleIcon,
	BlocksIcon,
	CarIcon,
	FlagIcon,
	HashIcon,
	LeafIcon,
	LightbulbIcon,
	SearchIcon,
	SmileIcon,
	UsersIcon,
	VolleyballIcon,
	XIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { buildEmojiData, emojiToValue, ICONS_CATEGORY } from "./emojiData";

const emojiData = buildEmojiData();
const resolveEmojiData = () => emojiData;

// Fetch the icon SVGs as soon as the picker chunk loads, so the Icons
// category does not render empty tiles while they download.
if (typeof Image !== "undefined") {
	for (const entry of emojiData.emojis) {
		if (entry.category === ICONS_CATEGORY) {
			new Image().src = entry.emoji;
		}
	}
}

const categoryIcons: Record<number, React.ReactNode> = {
	0: <SmileIcon />,
	1: <UsersIcon />,
	3: <LeafIcon />,
	4: <AppleIcon />,
	5: <CarIcon />,
	6: <VolleyballIcon />,
	7: <LightbulbIcon />,
	8: <HashIcon />,
	9: <FlagIcon />,
	[ICONS_CATEGORY]: <BlocksIcon />,
};

/** Frimousse renders one hidden sizer category; skip it by only matching
 * categories that are direct children of the list sizer. */
const CATEGORY_SELECTOR = "[frimousse-list-sizer] > [frimousse-category]";

const CategoryHeader: React.FC<EmojiPickerListCategoryHeaderProps> = ({
	category,
	...props
}) => (
	<div
		className="bg-surface-primary px-3 pt-3 pb-1.5 text-xs font-medium text-content-secondary"
		{...props}
	>
		{category.label}
	</div>
);

const Row: React.FC<EmojiPickerListRowProps> = ({ children, ...props }) => (
	<div className="scroll-my-1.5 px-1.5" {...props}>
		{children}
	</div>
);

const Emoji: React.FC<EmojiPickerListEmojiProps> = ({ emoji, ...props }) => (
	<button
		type="button"
		className={cn(
			"flex size-8 items-center justify-center rounded-md border-0 bg-transparent p-0",
			// The active emoji is the keyboard focus indicator: DOM focus stays
			// in the search input or viewport while arrow keys move it.
			"data-[active]:bg-surface-tertiary data-[active]:ring-2 data-[active]:ring-content-link data-[active]:ring-inset",
		)}
		{...props}
	>
		<ExternalImage
			src={emojiToValue(emoji.emoji)}
			className="pointer-events-none size-6 object-contain"
			decoding="async"
		/>
	</button>
);

const SkinToneButton: React.FC = () => {
	const [skinTone, setSkinTone, variations] = useSkinTone("\u270b");
	const index = variations.findIndex((v) => v.skinTone === skinTone);
	const current = variations[index];
	const next = variations[(index + 1) % variations.length];
	return (
		<InputGroupButton
			size="icon"
			aria-label={`Skin tone: ${skinTone === "none" ? "default" : skinTone}`}
			title="Change skin tone"
			onClick={() => next && setSkinTone(next.skinTone)}
		>
			{current && (
				<img alt="" src={emojiToValue(current.emoji)} className="size-4" />
			)}
		</InputGroupButton>
	);
};

type EmojiPickerProps = {
	/** Called with the icon path for the picked emoji or custom icon. */
	onEmojiSelect: (value: string) => void;
	autoFocus?: boolean;
};

const EmojiPicker: React.FC<EmojiPickerProps> = ({
	onEmojiSelect,
	autoFocus,
}) => {
	const viewportRef = useRef<HTMLDivElement>(null);
	const [search, setSearch] = useState("");
	const [activeCategory, setActiveCategory] = useState(0);

	const categoryElements = () =>
		Array.from(
			viewportRef.current?.querySelectorAll<HTMLElement>(CATEGORY_SELECTOR) ??
				[],
		);

	const handleScroll = (event: React.UIEvent<HTMLDivElement>) => {
		const top = event.currentTarget.scrollTop + 1;
		let index = 0;
		categoryElements().forEach((element, i) => {
			if (element.offsetTop <= top) {
				index = i;
			}
		});
		setActiveCategory(index);
	};

	const scrollToCategory = (index: number) => {
		setSearch("");
		setActiveCategory(index);
		// Clearing the search re-renders the full list on the next frame.
		requestAnimationFrame(() => {
			viewportRef.current?.scrollTo({
				top: categoryElements()[index]?.offsetTop ?? 0,
			});
		});
	};

	return (
		<Frimousse.Root
			className="isolate flex h-[400px] w-fit flex-col"
			columns={9}
			resolveEmojiData={resolveEmojiData}
			onEmojiSelect={({ emoji }) => onEmojiSelect(emojiToValue(emoji))}
		>
			<nav
				aria-label="Emoji categories"
				className="flex justify-between border-0 border-b border-solid border-border px-1.5"
			>
				{emojiData.categories.map((category, i) => {
					const isCurrent = search === "" && activeCategory === i;
					return (
						<button
							key={category.index}
							type="button"
							aria-label={category.label}
							aria-current={isCurrent || undefined}
							title={category.label}
							onClick={() => scrollToCategory(i)}
							className={cn(
								"flex size-8 cursor-pointer items-center justify-center rounded-md border-0 border-b-2 border-solid border-transparent bg-transparent p-0 text-content-secondary",
								"hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-inset",
								"aria-[current]:rounded-b-none aria-[current]:border-content-link aria-[current]:text-content-primary [&_svg]:size-4",
							)}
						>
							{categoryIcons[category.index]}
						</button>
					);
				})}
			</nav>
			<InputGroup className="mx-2 mt-2 w-auto">
				<InputGroupAddon>
					<SearchIcon className="size-icon-sm" />
				</InputGroupAddon>
				<Frimousse.Search
					autoFocus={autoFocus}
					value={search}
					onChange={(event) => setSearch(event.target.value)}
					placeholder="Search emojis and icons"
					aria-label="Search emojis and icons"
					// Hide the browser's native clear button in favor of the one below.
					className="h-9 min-w-0 flex-1 border-0 bg-transparent px-0 text-sm text-content-primary outline-hidden placeholder:text-content-secondary [&::-webkit-search-cancel-button]:appearance-none"
				/>
				<InputGroupAddon align="inline-end">
					{search !== "" && (
						<InputGroupButton
							size="icon"
							aria-label="Clear search"
							onClick={() => setSearch("")}
						>
							<XIcon />
						</InputGroupButton>
					)}
					<SkinToneButton />
				</InputGroupAddon>
			</InputGroup>
			<Frimousse.Viewport
				ref={viewportRef}
				// A tab stop for the grid: arrow keys move the active emoji while
				// focus is anywhere inside the picker.
				tabIndex={0}
				aria-label="Emojis and icons"
				onScroll={handleScroll}
				className="relative mt-1 flex-1 rounded-md outline-hidden focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-inset"
			>
				<Frimousse.Loading className="absolute inset-0 flex items-center justify-center text-sm text-content-secondary">
					Loading…
				</Frimousse.Loading>
				<Frimousse.Empty className="absolute inset-0 flex items-center justify-center text-sm text-content-secondary">
					No emojis or icons found
				</Frimousse.Empty>
				<Frimousse.List
					className="select-none pb-1.5"
					components={{ CategoryHeader, Row, Emoji }}
				/>
			</Frimousse.Viewport>
		</Frimousse.Root>
	);
};

export default EmojiPicker;
