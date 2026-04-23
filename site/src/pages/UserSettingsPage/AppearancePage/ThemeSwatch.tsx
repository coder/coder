import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ConcreteThemeName } from "#/theme/colorblind";
import { cn } from "#/utils/cn";
import { THEME_COPY } from "./themeCopy";

interface ThemeSwatchProps {
	theme: ConcreteThemeName;
	selected: boolean;
	onSelect: () => void;
}

/**
 * A small circular button used in sync-mode cards to pick which
 * concrete theme applies for a given OS color scheme. The swatch
 * renders a two-tone accent (git-added + git-deleted) over the
 * theme's surface-primary background so the user can tell the three
 * colorblind variants apart at a glance, without reading the label.
 *
 * The swatch resolves its colors by applying the theme's CSS class
 * to an inner wrapper, so the Tailwind tokens (`bg-git-added`, etc.)
 * pick up the theme-scoped CSS variables.
 */
export const ThemeSwatch: FC<ThemeSwatchProps> = ({
	theme,
	selected,
	onSelect,
}) => {
	const copy = THEME_COPY[theme];
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					role="radio"
					aria-checked={selected}
					aria-label={copy.title}
					onClick={onSelect}
					className={cn(
						"rounded-full size-8 p-0 border-2 border-solid cursor-pointer",
						"transition-[outline] outline outline-2 outline-offset-2",
						selected ? "outline-content-link" : "outline-transparent",
						"border-border-default",
						"focus-visible:outline-content-link focus-visible:outline-offset-2",
					)}
				>
					{/* The `theme` class scopes the CSS variables so the swatch
					    reads the palette of the theme it represents, not the
					    currently applied one. */}
					<span
						className={cn(
							theme,
							"block size-full rounded-full overflow-hidden",
							"bg-surface-primary relative",
						)}
					>
						<span
							className="absolute inset-0 bg-git-added"
							style={{ clipPath: "polygon(0 0, 100% 0, 0 100%)" }}
						/>
						<span
							className="absolute inset-0 bg-git-deleted"
							style={{ clipPath: "polygon(100% 0, 100% 100%, 0 100%)" }}
						/>
					</span>
				</button>
			</TooltipTrigger>
			<TooltipContent sideOffset={8}>
				<div className="flex flex-col gap-0.5">
					<span className="font-medium text-content-primary">{copy.title}</span>
					<span className="text-content-secondary">{copy.description}</span>
				</div>
			</TooltipContent>
		</Tooltip>
	);
};
