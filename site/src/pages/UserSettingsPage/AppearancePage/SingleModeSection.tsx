import type { FC } from "react";
import { PreviewBadge } from "#/components/Badges/Badges";
import type { ConcreteThemeName } from "#/theme/colorblind";
import { CONCRETE_THEMES } from "#/theme/colorblind";
import { cn } from "#/utils/cn";
import { ThemePreview } from "./ThemePreview";
import { THEME_COPY } from "./themeCopy";

interface SingleModeSectionProps {
	selected: ConcreteThemeName;
	onSelect: (theme: ConcreteThemeName) => void;
}

// Row-major order matching the design screenshot: light row first,
// then dark row. The default variant in each row leads, followed by
// the colorblind variants.
const SINGLE_MODE_ORDER: ConcreteThemeName[] = [
	"light",
	"light-protan-deuter",
	"light-tritan",
	"dark",
	"dark-protan-deuter",
	"dark-tritan",
];

export const SingleModeSection: FC<SingleModeSectionProps> = ({
	selected,
	onSelect,
}) => {
	return (
		<div
			role="radiogroup"
			aria-label="Theme"
			className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3"
		>
			{SINGLE_MODE_ORDER.map((theme) => (
				<SingleTile
					key={theme}
					theme={theme}
					selected={theme === selected}
					onSelect={() => onSelect(theme)}
				/>
			))}
		</div>
	);
};

interface SingleTileProps {
	theme: ConcreteThemeName;
	selected: boolean;
	onSelect: () => void;
}

const SingleTile: FC<SingleTileProps> = ({ theme, selected, onSelect }) => {
	const copy = THEME_COPY[theme];
	return (
		<label
			className={cn(
				"flex cursor-pointer flex-col gap-3 rounded-md border border-solid border-border p-4",
				selected &&
					"border-content-link outline outline-2 outline-content-link",
			)}
		>
			<input
				type="radio"
				name="theme-single"
				value={theme}
				checked={selected}
				onChange={onSelect}
				className="sr-only"
			/>
			<div className="relative">
				<ThemePreview theme={theme} size="lg" />
				{copy.beta && (
					<div className="absolute right-2 top-2">
						<PreviewBadge />
					</div>
				)}
			</div>
			<div className="flex items-start gap-3">
				<span
					aria-hidden
					className={cn(
						"mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border border-solid",
						selected
							? "border-content-link bg-content-link"
							: "border-border-secondary",
					)}
				>
					{selected && (
						<span className="size-1.5 rounded-full bg-surface-primary" />
					)}
				</span>
				<div className="flex flex-col gap-1">
					<span className="font-medium text-content-primary">{copy.title}</span>
					<span className="text-sm text-content-secondary">
						{copy.description}
					</span>
				</div>
			</div>
		</label>
	);
};

// Sanity: every concrete theme must have a tile. Asserted at module
// load time so a future theme addition without a matching entry in
// SINGLE_MODE_ORDER is impossible to ship.
if (SINGLE_MODE_ORDER.length !== CONCRETE_THEMES.length) {
	throw new Error(
		"SINGLE_MODE_ORDER is out of sync with CONCRETE_THEMES. Update both together.",
	);
}
