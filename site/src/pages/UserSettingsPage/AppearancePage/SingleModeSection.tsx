import { cn } from "cn";
import { useId } from "react";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import type { ConcreteThemeName } from "#/theme";
import { ThemePreview } from "./ThemePreview";
import { DARK_THEMES, LIGHT_THEMES, THEME_COPY } from "./themeCopy";

type SingleModeSectionProps = {
	selected: ConcreteThemeName;
	name?: string;
	onSelect: (theme: ConcreteThemeName) => void;
};

const SINGLE_MODE_ORDER: ConcreteThemeName[] = [
	...LIGHT_THEMES,
	...DARK_THEMES,
];

export const SingleModeSection: React.FC<SingleModeSectionProps> = ({
	selected,
	name = "theme-single",
	onSelect,
}) => {
	return (
		<RadioGroup
			aria-label="Theme"
			name={name}
			value={selected}
			onValueChange={(value) => {
				const theme = SINGLE_MODE_ORDER.find((t) => t === value);
				if (theme) {
					onSelect(theme);
				}
			}}
			className="grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3"
		>
			{SINGLE_MODE_ORDER.map((theme) => (
				<SingleTile key={theme} theme={theme} selected={theme === selected} />
			))}
		</RadioGroup>
	);
};

type SingleTileProps = {
	theme: ConcreteThemeName;
	selected: boolean;
};

const SingleTile: React.FC<SingleTileProps> = ({ theme, selected }) => {
	const copy = THEME_COPY[theme];
	const id = useId();
	return (
		<label
			htmlFor={id}
			className={cn(
				"flex cursor-pointer flex-col gap-3 rounded-md border border-solid border-border p-4",
				selected && "ring-2 ring-content-link",
				"has-[button:focus-visible]:outline-solid has-[button:focus-visible]:outline-2 has-[button:focus-visible]:outline-offset-2 has-[button:focus-visible]:outline-content-link",
			)}
		>
			<div className="relative">
				<ThemePreview theme={theme} size="lg" />
			</div>
			<div className="flex items-start gap-3">
				<RadioGroupItem id={id} value={theme} className="mt-0.5 shrink-0" />
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
