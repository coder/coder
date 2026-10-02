import { cn } from "cn";
import { useId, useState } from "react";
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

type BannerColorPickerProps = {
	color: string;
	onChange: (color: string) => void;
};

export const BannerColorPicker: React.FC<BannerColorPickerProps> = ({
	color,
	onChange,
}) => {
	const [hexDraft, setHexDraft] = useState<string | undefined>(undefined);
	const hexId = useId();

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
							onClick={() => {
								setHexDraft(undefined);
								onChange(preset);
							}}
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
						value={hexDraft ?? color.replace(/^#/, "")}
						onChange={(event) => {
							const digits = event.target.value.replaceAll("#", "").slice(0, 6);
							setHexDraft(digits);
							const next = `#${digits}`;
							if (isHexColor(next)) {
								onChange(next);
							}
						}}
						onBlur={() => setHexDraft(undefined)}
						autoComplete="off"
						spellCheck={false}
						className="font-mono"
					/>
				</InputGroup>
			</div>
		</div>
	);
};
