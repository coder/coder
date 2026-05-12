import type { ChangeEventHandler, FocusEventHandler, ReactNode } from "react";
import { type FC, lazy, Suspense, useState } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

type IconFieldProps = {
	onPickEmoji: (value: string) => void;
	label?: string;
	name?: string;
	value?: string | number;
	onChange?: ChangeEventHandler<HTMLInputElement | HTMLTextAreaElement>;
	onBlur?: FocusEventHandler<HTMLInputElement | HTMLTextAreaElement>;
	error?: boolean;
	helperText?: ReactNode;
	id?: string;
	disabled?: boolean;
	/** @deprecated Accepted for backward compatibility but ignored. */
	fullWidth?: boolean;
};

const EmojiPicker = lazy(() => import("./EmojiPicker"));

export const IconField: FC<IconFieldProps> = ({
	onPickEmoji,
	label = "Icon",
	name,
	value,
	onChange,
	onBlur,
	error,
	helperText,
	id,
	disabled,
}) => {
	if (
		typeof value !== "string" &&
		typeof value !== "undefined" &&
		typeof value !== "number"
	) {
		throw new Error(`Invalid icon value "${typeof value}"`);
	}

	const stringValue = typeof value === "number" ? String(value) : value;
	const hasIcon = stringValue && stringValue !== "";
	const [open, setOpen] = useState(false);

	return (
		<div className="flex flex-col gap-2">
			{label && <Label htmlFor={id}>{label}</Label>}
			<div className="flex items-center gap-2">
				<div className="relative flex-1">
					<Input
						id={id}
						name={name}
						value={stringValue}
						onChange={onChange}
						onBlur={onBlur}
						disabled={disabled}
						aria-invalid={error}
						className={cn(
							error && "border-border-destructive",
							hasIcon && "pr-10",
						)}
					/>
					{hasIcon && (
						<div className="absolute right-3 top-1/2 -translate-y-1/2 w-5 h-5 flex items-center justify-center [&_img]:max-w-full [&_img]:object-contain">
							<ExternalImage
								alt=""
								src={stringValue}
								// Prevent browser from displaying the ugly error icon if the
								// image path is wrong or user didn't finish typing the url
								onError={(e) => {
									e.currentTarget.style.display = "none";
								}}
								onLoad={(e) => {
									e.currentTarget.style.display = "inline";
								}}
							/>
						</div>
					)}
				</div>

				<Popover open={open} onOpenChange={setOpen}>
					<PopoverTrigger asChild>
						<Button variant="outline" size="lg" className="group flex-shrink-0">
							Emoji
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
									const emojiValue =
										emoji.src ?? `/emojis/${emoji.unified}.png`;
									onPickEmoji(emojiValue);
									setOpen(false);
								}}
							/>
						</Suspense>
					</PopoverContent>
				</Popover>

				{/* Pre-warm the emoji picker so users don't wait for it to load.
				Except in tests, where it would slow things down. */}
				{process.env.NODE_ENV !== "test" && (
					<div className="sr-only" aria-hidden="true">
						<Suspense>
							<EmojiPicker onEmojiSelect={() => {}} />
						</Suspense>
					</div>
				)}
			</div>
			{error && helperText ? (
				<span className="text-xs text-content-destructive">{helperText}</span>
			) : (
				helperText && (
					<span className="text-xs text-content-secondary">{helperText}</span>
				)
			)}
		</div>
	);
};
