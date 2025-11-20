import { useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import MiniTooltip from "components/MiniTooltip/MiniTooltip";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { SearchIcon, XIcon } from "lucide-react";
import { type FC, type ReactNode, useMemo, useState } from "react";
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

const IconsPage: FC = () => {
	const theme = useTheme();
	const [searchInputText, setSearchInputText] = useState("");
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
						<MiniTooltip
							side="bottom"
							align="end"
							title="You can suggest a new icon by submitting a Pull Request to our public GitHub repository. Just keep in mind that it should be relevant to many Coder users, and redistributable under a permissive license."
						>
							<Link href="https://github.com/coder/coder/tree/main/site/static/icon">
								Suggest an icon
							</Link>
						</MiniTooltip>
					}
				>
					<PageHeaderTitle>Icons</PageHeaderTitle>
					<PageHeaderSubtitle>
						All of the icons included with Coder
					</PageHeaderSubtitle>
				</PageHeader>
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
								<SearchIcon className="size-icon-xs text-content-secondary" />
							</InputAdornment>
						),
						endAdornment: searchInputText && (
							<InputAdornment position="end">
								<MiniTooltip title="Clear filter">
									<IconButton
										size="small"
										onClick={() => setSearchInputText("")}
									>
										<XIcon className="size-icon-xs" />
									</IconButton>
								</MiniTooltip>
							</InputAdornment>
						),
					}}
				/>

				<div className="flex flex-row gap-2 justify-center flex-wrap max-w-full mt-8">
					{searchedIcons.length === 0 && (
						<EmptyState message="No results matched your search" />
					)}
					{searchedIcons.map((icon) => (
						<CopyableValue key={icon.url} value={icon.url}>
							<div className="flex flex-col gap-4 items-center max-w-full p-3">
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
							</div>
						</CopyableValue>
					))}
				</div>
			</Margins>
		</>
	);
};

export default IconsPage;
