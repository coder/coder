import { MoonIcon, SunIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import type { ConcreteThemeName } from "#/theme/colorblind";
import { cn } from "#/utils/cn";
import { ThemePreview } from "./ThemePreview";
import { ThemeSwatch } from "./ThemeSwatch";
import { DARK_THEMES, LIGHT_THEMES } from "./themeCopy";

interface SyncModeSectionProps {
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
	/**
	 * The OS color scheme currently in effect. Used to decide which
	 * card shows the "Active" pill so the user knows which theme they
	 * are looking at right now.
	 */
	activeScheme: "dark" | "light";
	onSelect: (scheme: "light" | "dark", theme: ConcreteThemeName) => void;
}

export const SyncModeSection: FC<SyncModeSectionProps> = ({
	light,
	dark,
	activeScheme,
	onSelect,
}) => {
	return (
		<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
			<SyncCard
				scheme="light"
				selected={light}
				active={activeScheme === "light"}
				onSelect={(theme) => onSelect("light", theme)}
			/>
			<SyncCard
				scheme="dark"
				selected={dark}
				active={activeScheme === "dark"}
				onSelect={(theme) => onSelect("dark", theme)}
			/>
		</div>
	);
};

interface SyncCardProps {
	scheme: "light" | "dark";
	selected: ConcreteThemeName;
	active: boolean;
	onSelect: (theme: ConcreteThemeName) => void;
}

const SyncCard: FC<SyncCardProps> = ({
	scheme,
	selected,
	active,
	onSelect,
}) => {
	const themes = scheme === "light" ? LIGHT_THEMES : DARK_THEMES;
	const Icon = scheme === "light" ? SunIcon : MoonIcon;
	const title = scheme === "light" ? "Light theme" : "Dark theme";
	const description =
		scheme === "light"
			? 'This theme will be active when your system is set to "light mode".'
			: 'This theme will be active when your system is set to "dark mode".';

	return (
		<div
			className={cn(
				"flex flex-col gap-4 rounded-md border border-solid border-border p-4",
				active && "border-content-link",
			)}
		>
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-2">
					<Icon className="size-icon-sm text-content-secondary" />
					<span className="font-medium text-content-primary">{title}</span>
				</div>
				{active && (
					<Badge variant="info" size="sm">
						Active
					</Badge>
				)}
			</div>
			<p className="m-0 text-sm text-content-secondary">{description}</p>
			<ThemePreview theme={selected} size="lg" />
			<div
				role="radiogroup"
				aria-label={`${title} options`}
				className="flex items-center gap-3"
			>
				{themes.map((theme) => (
					<ThemeSwatch
						key={theme}
						theme={theme}
						selected={theme === selected}
						onSelect={() => onSelect(theme)}
					/>
				))}
			</div>
		</div>
	);
};
