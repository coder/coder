import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { baseModeFor, type ConcreteThemeName } from "#/theme";
import { cn } from "#/utils/cn";
import { THEME_COPY } from "./themeCopy";

interface ThemeSwatchProps {
	name: string;
	theme: ConcreteThemeName;
	selected: boolean;
	onSelect: () => void;
	onPreview?: () => void;
	onPreviewEnd?: () => void;
}

export const ThemeSwatch: FC<ThemeSwatchProps> = ({
	name,
	theme,
	selected,
	onSelect,
	onPreview,
	onPreviewEnd,
}) => {
	const copy = THEME_COPY[theme];
	const isDefaultTheme = theme === "light" || theme === "dark";
	const accentClass = theme.includes("protan-deuter")
		? "bg-[#bf8700]"
		: "bg-[#cf222e]";
	return (
		<Tooltip delayDuration={1000}>
			<TooltipTrigger asChild>
				<label
					className={cn(
						"inline-flex rounded-full size-8 p-0 border-2 border-solid cursor-pointer",
						"transition-[outline] outline outline-2 outline-offset-2",
						selected ? "outline-content-link" : "outline-transparent",
						"border-border-default",
						"has-[input:focus-visible]:outline-content-link has-[input:focus-visible]:outline-offset-2",
					)}
					onMouseEnter={onPreview}
					onMouseLeave={onPreviewEnd}
					onFocus={onPreview}
					onBlur={onPreviewEnd}
				>
					<input
						type="radio"
						name={name}
						value={theme}
						checked={selected}
						onChange={onSelect}
						aria-label={copy.title}
						className="sr-only"
					/>
					<span
						className={cn(
							baseModeFor(theme),
							theme,
							"block size-full rounded-full overflow-hidden",
							"bg-surface-primary relative",
						)}
					>
						{!isDefaultTheme && (
							<>
								<span
									className="absolute inset-0 bg-[#0969da]"
									style={{
										clipPath: "polygon(0 100%, 50% 50%, 100% 100%)",
									}}
								/>
								<span
									className={cn("absolute inset-0", accentClass)}
									style={{
										clipPath: "polygon(100% 0, 100% 100%, 50% 50%)",
									}}
								/>
							</>
						)}
					</span>
				</label>
			</TooltipTrigger>
			<TooltipContent
				side="bottom"
				sideOffset={8}
				className="text-content-primary"
			>
				{copy.title}
			</TooltipContent>
		</Tooltip>
	);
};
