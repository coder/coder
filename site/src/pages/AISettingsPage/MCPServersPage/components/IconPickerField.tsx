import { type FC, lazy, Suspense, useState } from "react";
import { ChevronDownIcon as AnimatedChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Loader } from "#/components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";

const EmojiPicker = lazy(() => import("#/components/IconField/EmojiPicker"));

interface IconPickerFieldProps {
	id?: string;
	value: string;
	placeholder?: string;
	disabled?: boolean;
	onChange: (value: string) => void;
}

export const IconPickerField: FC<IconPickerFieldProps> = ({
	id,
	value,
	placeholder,
	disabled,
	onChange,
}) => {
	const [open, setOpen] = useState(false);
	const hasIcon = value !== "";

	return (
		<InputGroup>
			<InputGroupInput
				id={id}
				value={value}
				onChange={(event) => onChange(event.target.value)}
				placeholder={placeholder}
				disabled={disabled}
				className="min-w-0 placeholder:text-content-disabled"
				spellCheck={false}
			/>
			<InputGroupAddon align="inline-end" className="gap-1.5">
				{hasIcon && (
					<span className="flex size-5 items-center justify-center [&_img]:max-w-full [&_img]:object-contain">
						<ExternalImage
							alt=""
							src={value}
							onError={(event) => {
								event.currentTarget.style.display = "none";
							}}
							onLoad={(event) => {
								event.currentTarget.style.display = "inline";
							}}
						/>
					</span>
				)}
				<Popover open={open} onOpenChange={setOpen}>
					<PopoverTrigger asChild>
						<Button
							type="button"
							variant="subtle"
							size="sm"
							className="group h-7 gap-1"
							disabled={disabled}
							aria-label="Pick an emoji or icon"
						>
							Emoji
							<AnimatedChevronDownIcon />
						</Button>
					</PopoverTrigger>
					<PopoverContent
						side="bottom"
						align="end"
						className="w-min"
						// The popover is portaled in the DOM but still a React child of
						// InputGroupAddon, whose click handler focuses the text input.
						// Stop clicks here so the emoji picker keeps focus.
						onClick={(event) => event.stopPropagation()}
					>
						<Suspense fallback={<Loader />}>
							<EmojiPicker
								onEmojiSelect={(emoji) => {
									const picked = emoji.src ?? `/emojis/${emoji.unified}.png`;
									onChange(picked);
									setOpen(false);
								}}
							/>
						</Suspense>
					</PopoverContent>
				</Popover>
			</InputGroupAddon>
		</InputGroup>
	);
};
