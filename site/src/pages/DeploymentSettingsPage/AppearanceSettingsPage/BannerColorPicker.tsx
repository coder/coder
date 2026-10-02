import { cn } from "cn";
import { hex, hsl } from "color-convert";
import { useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { isHexColor } from "#/utils/colors";

const PRESET_COLORS = [
	"#8b5cf6",
	"#d94a5d",
	"#f78da7",
	"#d65d0f",
	"#ff6900",
	"#fcb900",
	"#0693e3",
	"#8ed1fc",
	"#4cd473",
	"#abb8c3",
] as const;

// Lightness choices at 50% saturation, so a hue can become a usable banner color.
const LIGHTNESS_STOPS = [80, 65, 50, 35, 20] as const;

const HUE_GRADIENT =
	"linear-gradient(to right, hsl(0 100% 50%), hsl(60 100% 50%), hsl(120 100% 50%), hsl(180 100% 50%), hsl(240 100% 50%), hsl(300 100% 50%), hsl(360 100% 50%))";

type BannerColorPickerProps = {
	color: string;
	onChange: (color: string) => void;
};

type Channels = {
	h: number;
	s: number;
	l: number;
};

export const BannerColorPicker: React.FC<BannerColorPickerProps> = ({
	color,
	onChange,
}) => {
	const [showSlider, setShowSlider] = useState(false);
	const [hexDraft, setHexDraft] = useState<string | undefined>(undefined);
	const hexId = useId();
	const hueId = useId();
	const channels = readChannels(color);

	const apply = (next: string) => {
		setHexDraft(undefined);
		onChange(next);
	};

	return (
		<div className="flex flex-col gap-4">
			{showSlider ? (
				<HueSlider
					hueId={hueId}
					color={color}
					channels={channels}
					onChange={apply}
				/>
			) : (
				<Palette
					hexId={hexId}
					color={color}
					hexValue={hexDraft ?? color.replace(/^#/, "")}
					onHexChange={(digits) => {
						setHexDraft(digits);
						const next = `#${digits}`;
						if (isHexColor(next)) {
							onChange(next);
						}
					}}
					onHexBlur={() => setHexDraft(undefined)}
					onSwatch={apply}
				/>
			)}
			<div>
				<Button variant="outline" onClick={() => setShowSlider((it) => !it)}>
					Show {showSlider ? "palette" : "slider"}
				</Button>
			</div>
		</div>
	);
};

type PaletteProps = {
	hexId: string;
	color: string;
	hexValue: string;
	onHexChange: (digits: string) => void;
	onHexBlur: () => void;
	onSwatch: (color: string) => void;
};

const Palette: React.FC<PaletteProps> = ({
	hexId,
	color,
	hexValue,
	onHexChange,
	onHexBlur,
	onSwatch,
}) => {
	return (
		<div className="flex flex-col gap-3">
			<div className="flex flex-wrap gap-1.5">
				{PRESET_COLORS.map((preset) => {
					const selected = preset.toLowerCase() === color.toLowerCase();
					return (
						<button
							key={preset}
							type="button"
							aria-label={preset}
							aria-pressed={selected}
							className={cn(
								"size-8 rounded-md border border-solid border-border p-0",
								"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
								selected &&
									"ring-2 ring-content-link ring-offset-2 ring-offset-surface-primary",
							)}
							style={{ backgroundColor: preset }}
							onClick={() => onSwatch(preset)}
						/>
					);
				})}
			</div>
			<div className="flex items-center gap-2">
				<Label htmlFor={hexId}>Hex</Label>
				<InputGroup className="w-36">
					<InputGroupAddon>#</InputGroupAddon>
					<InputGroupInput
						id={hexId}
						value={hexValue}
						onChange={(event) => {
							onHexChange(event.target.value.replaceAll("#", "").slice(0, 6));
						}}
						onBlur={onHexBlur}
						autoComplete="off"
						spellCheck={false}
						className="font-mono"
					/>
				</InputGroup>
			</div>
		</div>
	);
};

type HueSliderProps = {
	hueId: string;
	color: string;
	channels: Channels;
	onChange: (color: string) => void;
};

const HueSlider: React.FC<HueSliderProps> = ({
	hueId,
	color,
	channels,
	onChange,
}) => {
	const hue = clamp(Math.round(channels.h), 0, 359);

	return (
		<div className="flex flex-col gap-3">
			<div className="flex flex-col gap-2">
				<Label htmlFor={hueId}>Hue</Label>
				<div className="relative h-4">
					<div
						aria-hidden
						className="pointer-events-none absolute inset-x-0 top-1/2 h-3 -translate-y-1/2 rounded-sm"
						style={{ backgroundImage: HUE_GRADIENT }}
					/>
					<input
						id={hueId}
						type="range"
						min={0}
						max={359}
						step={1}
						value={hue}
						onChange={(event) => {
							const nextHue = Number(event.target.value);
							if (!Number.isInteger(nextHue) || nextHue < 0 || nextHue > 359) {
								return;
							}
							onChange(channelsToHex(nextHue, channels.s, channels.l));
						}}
						className={cn(
							"relative h-4 w-full cursor-pointer appearance-none bg-transparent",
							"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
							"[&::-webkit-slider-thumb]:size-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:bg-transparent",
							"[&::-moz-range-thumb]:size-4 [&::-moz-range-thumb]:appearance-none [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-transparent",
						)}
					/>
					<span
						aria-hidden
						className="pointer-events-none absolute top-0 size-4 rounded-full border-2 border-surface-primary shadow-sm"
						style={{
							backgroundColor: color,
							left: `calc(${hue} / 359 * (100% - 1rem))`,
						}}
					/>
				</div>
			</div>
			<div className="flex gap-px">
				{LIGHTNESS_STOPS.map((stop) => {
					const swatch = channelsToHex(channels.h, 50, stop);
					const selected =
						Math.abs(channels.s - 50) < 10 && Math.abs(channels.l - stop) < 10;
					return (
						<button
							key={stop}
							type="button"
							aria-label={`${stop}% lightness`}
							aria-pressed={selected}
							className={cn(
								"h-6 flex-1 border-0 p-0",
								"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
								selected && "ring-2 ring-content-primary",
							)}
							style={{ backgroundColor: swatch }}
							onClick={() => onChange(swatch)}
						/>
					);
				})}
			</div>
		</div>
	);
};

function readChannels(color: string): Channels {
	if (!isHexColor(color)) {
		return { h: 0, s: 0, l: 70 };
	}
	const [h, s, l] = hex.hsl(color.slice(1));
	if (![h, s, l].every((channel) => Number.isFinite(channel))) {
		return { h: 0, s: 0, l: 70 };
	}
	return { h, s, l };
}

function channelsToHex(h: number, s: number, l: number): string {
	return `#${hsl
		.hex([
			clamp(Math.round(h), 0, 359),
			clamp(Math.round(s), 0, 100),
			clamp(Math.round(l), 0, 100),
		])
		.toLowerCase()}`;
}

function clamp(value: number, min: number, max: number): number {
	if (!Number.isFinite(value)) {
		return min;
	}
	return Math.min(max, Math.max(min, value));
}
