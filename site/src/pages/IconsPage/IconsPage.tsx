import { css, Global, useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import { Button } from "components/Button/Button";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";
import { ChevronDownIcon, SearchIcon, XIcon } from "lucide-react";
import {
	type FC,
	lazy,
	type ReactNode,
	Suspense,
	useMemo,
	useState,
} from "react";
import {
	defaultParametersForBuiltinIcons,
	parseImageParameters,
} from "theme/externalImages";
import icons from "theme/icons.json";
import uFuzzy from "ufuzzy";
import { pageTitle } from "utils/page";

const iconsWithoutSuffix = icons.map((icon) => icon.split(".")[0]);
const fuzzyFinder = new uFuzzy({
	intraMode: 1,
	intraIns: 1,
	intraSub: 1,
	intraTrn: 1,
	intraDel: 1,
});

// See: https://github.com/missive/emoji-mart/issues/51#issuecomment-287353222
const urlFromUnifiedCode = (unified: string) =>
	`/emojis/${unified.replace(/-fe0f$/, "")}.png`;

const EmojiPicker = lazy(() => import("components/IconField/EmojiPicker"));

const IconsPage: FC = () => {
	const theme = useTheme();
	const [searchInputText, setSearchInputText] = useState("");
	const [emojiPickerOpen, setEmojiPickerOpen] = useState(false);
	const [selectedEmojiUrl, setSelectedEmojiUrl] = useState("");
	const searchText = searchInputText.trim();

	const searchedIcons = useMemo(() => {
		if (!searchText) {
			return icons.map((icon) => ({ url: `/icon/${icon}`, description: icon }));
		}

		const [map, info, sorted] = fuzzyFinder.search(
			iconsWithoutSuffix,
			searchText,
		);

		// We hit an invalid state somehow
		if (!map || !info || !sorted) {
			return [];
		}

		return sorted.map((i) => {
			const iconName = icons[info.idx[i]];
			const ranges = info.ranges[i];

			const nodes: ReactNode[] = [];
			let cursor = 0;
			for (let j = 0; j < ranges.length; j += 2) {
				nodes.push(iconName.slice(cursor, ranges[j]));
				nodes.push(
					<mark key={j + 1}>{iconName.slice(ranges[j], ranges[j + 1])}</mark>,
				);
				cursor = ranges[j + 1];
			}
			nodes.push(iconName.slice(cursor));
			return { url: `/icon/${iconName}`, description: nodes };
		});
	}, [searchText]);

	return (
		<>
			<title>{pageTitle("Icons")}</title>
			<Margins>
				<PageHeader
					actions={
						<div className="flex items-center gap-4">
							<Tooltip
								placement="bottom-end"
								title={
									<p
										css={{
											padding: 8,
											fontSize: 13,
											lineHeight: 1.5,
										}}
									>
										You can suggest a new icon by submitting a Pull Request to
										our public GitHub repository. Just keep in mind that it
										should be relevant to many Coder users, and redistributable
										under a permissive license.
									</p>
								}
							>
								<Link href="https://github.com/coder/coder/tree/main/site/static/icon">
									Suggest an icon
								</Link>
							</Tooltip>

							<Global
								styles={css`
									em-emoji-picker {
										--rgb-background: ${theme.palette.background.paper};
										--rgb-input: ${theme.palette.primary.main};
										--rgb-color: ${theme.palette.text.primary};
									}
								`}
							/>
							<Popover open={emojiPickerOpen} onOpenChange={setEmojiPickerOpen}>
								<PopoverTrigger asChild>
									<Button variant="outline" size="lg" className="flex-shrink-0">
										Emoji Picker
										<ChevronDownIcon />
									</Button>
								</PopoverTrigger>
								<PopoverContent
									id="emoji"
									side="bottom"
									align="end"
									className="w-min"
								>
									<Suspense fallback={<Loader />}>
										<EmojiPicker
											onEmojiSelect={(emoji) => {
												const value =
													emoji.src ?? urlFromUnifiedCode(emoji.unified);
												setSelectedEmojiUrl(value);
												setEmojiPickerOpen(false);
											}}
										/>
									</Suspense>
								</PopoverContent>
							</Popover>
						</div>
					}
				>
					<PageHeaderTitle>Icons</PageHeaderTitle>
					<PageHeaderSubtitle>
						All of the icons included with Coder
					</PageHeaderSubtitle>
				</PageHeader>

				{selectedEmojiUrl && (
					<div
						css={{
							marginTop: 16,
							marginBottom: 16,
							padding: 16,
							backgroundColor: theme.palette.background.paper,
							borderRadius: 8,
							border: `1px solid ${theme.palette.divider}`,
						}}
					>
						<Stack direction="row" alignItems="center" spacing={2}>
							<img
								src={selectedEmojiUrl}
								alt="Selected emoji"
								css={{
									width: 40,
									height: 40,
									objectFit: "contain",
								}}
							/>
							<div css={{ flex: 1 }}>
								<div
									css={{
										fontSize: 12,
										color: theme.palette.text.secondary,
										marginBottom: 4,
									}}
								>
									Selected Emoji URL
								</div>
								<CopyableValue value={selectedEmojiUrl}>
									<code
										css={{
											fontSize: 14,
											color: theme.palette.text.primary,
											backgroundColor: theme.palette.background.default,
											padding: "4px 8px",
											borderRadius: 4,
											display: "inline-block",
										}}
									>
										{selectedEmojiUrl}
									</code>
								</CopyableValue>
							</div>
							<IconButton
								size="small"
								onClick={() => setSelectedEmojiUrl("")}
								css={{ color: theme.palette.text.secondary }}
							>
								<XIcon className="size-icon-xs" />
							</IconButton>
						</Stack>
					</div>
				)}
				<TextField
					size="small"
					InputProps={{
						"aria-label": "Filter",
						name: "query",
						placeholder: "Search…",
						value: searchInputText,
						onChange: (event) => setSearchInputText(event.target.value),
						sx: {
							borderRadius: "6px",
							marginLeft: "-1px",
							"& input::placeholder": {
								color: theme.palette.text.secondary,
							},
							"& .MuiInputAdornment-root": {
								marginLeft: 0,
							},
						},
						startAdornment: (
							<InputAdornment position="start">
								<SearchIcon
									className="size-icon-xs"
									css={{
										color: theme.palette.text.secondary,
									}}
								/>
							</InputAdornment>
						),
						endAdornment: searchInputText && (
							<InputAdornment position="end">
								<Tooltip title="Clear filter">
									<IconButton
										size="small"
										onClick={() => setSearchInputText("")}
									>
										<XIcon className="size-icon-xs" />
									</IconButton>
								</Tooltip>
							</InputAdornment>
						),
					}}
				/>

				<Stack
					direction="row"
					wrap="wrap"
					spacing={1}
					justifyContent="center"
					css={{ marginTop: 32 }}
				>
					{searchedIcons.length === 0 && (
						<EmptyState message="No results matched your search" />
					)}
					{searchedIcons.map((icon) => (
						<CopyableValue key={icon.url} value={icon.url} placement="bottom">
							<Stack alignItems="center" css={{ margin: 12 }}>
								<img
									alt={icon.url}
									src={icon.url}
									css={[
										{
											width: 60,
											height: 60,
											objectFit: "contain",
											pointerEvents: "none",
											padding: 12,
										},
										parseImageParameters(
											theme.externalImages,
											defaultParametersForBuiltinIcons.get(icon.url) ?? "",
										),
									]}
								/>
								<figcaption
									css={{
										width: 88,
										height: 48,
										fontSize: 13,
										textOverflow: "ellipsis",
										textAlign: "center",
										overflow: "hidden",
									}}
								>
									{icon.description}
								</figcaption>
							</Stack>
						</CopyableValue>
					))}
				</Stack>
			</Margins>
		</>
	);
};

export default IconsPage;
