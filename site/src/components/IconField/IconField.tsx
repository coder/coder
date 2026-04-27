import { css, Global, useTheme } from "@emotion/react";
import {
	type ChangeEventHandler,
	type FC,
	type FocusEventHandler,
	lazy,
	type ReactNode,
	Suspense,
	useId,
	useState,
} from "react";
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
	value?: string | number;
	onChange?: ChangeEventHandler<HTMLInputElement | HTMLTextAreaElement>;
	onBlur?: FocusEventHandler<HTMLInputElement | HTMLTextAreaElement>;
	name?: string;
	id?: string;
	disabled?: boolean;
	fullWidth?: boolean;
	label?: ReactNode;
	error?: boolean;
	helperText?: ReactNode;
	className?: string;
	onPickEmoji: (value: string) => void;
};

const EmojiPicker = lazy(() => import("./EmojiPicker"));

export const IconField: FC<IconFieldProps> = ({
	className,
	disabled,
	error,
	fullWidth = true,
	helperText,
	id,
	label = "Icon",
	name,
	onBlur,
	onChange,
	onPickEmoji,
	value,
}) => {
	if (typeof value !== "string" && typeof value !== "undefined") {
		throw new Error(`Invalid icon value "${typeof value}"`);
	}

	const theme = useTheme();
	const generatedId = useId();
	const inputId = id ?? generatedId;
	const helperTextId = helperText ? `${inputId}-helper-text` : undefined;
	const hasIcon = value !== undefined && value !== "";
	const [open, setOpen] = useState(false);

	return (
		<div
			className={cn("flex flex-col gap-2", fullWidth && "w-full", className)}
		>
			<Label htmlFor={inputId}>{label}</Label>
			<div className="flex items-center gap-2">
				<div className={cn("relative", fullWidth && "w-full")}>
					<Input
						id={inputId}
						name={name}
						value={value}
						onChange={onChange}
						onBlur={onBlur}
						disabled={disabled}
						aria-invalid={error || undefined}
						aria-describedby={helperTextId}
						className={cn(!fullWidth && "w-auto", hasIcon && "pr-12")}
					/>
					{hasIcon && (
						<div className="pointer-events-none absolute inset-y-0 right-3 flex items-center">
							<div className="w-6 h-6 flex items-center justify-center [&_img]:max-w-full [&_img]:object-contain">
								<ExternalImage
									alt=""
									src={value}
									// This prevent browser to display the ugly error icon if the
									// image path is wrong or user didn't finish typing the url
									onError={(e) => {
										e.currentTarget.style.display = "none";
									}}
									onLoad={(e) => {
										e.currentTarget.style.display = "inline";
									}}
								/>
							</div>
						</div>
					)}
				</div>

				<Global
					styles={css`
						em-emoji-picker {
							--rgb-background: ${theme.palette.background.paper};
							--rgb-input: ${theme.palette.primary.main};
							--rgb-color: ${theme.palette.text.primary};
						}
					`}
				/>
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
									const value = emoji.src ?? `/emojis/${emoji.unified}.png`;
									onPickEmoji(value);
									setOpen(false);
								}}
							/>
						</Suspense>
					</PopoverContent>
				</Popover>
			</div>

			{helperText && (
				<p
					id={helperTextId}
					className={cn(
						"m-0 text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</p>
			)}

			{/*
      - This component takes a long time to load (easily several seconds), so we
      don't want to wait until the user actually clicks the button to start loading.
      Unfortunately, React doesn't provide an API to start warming a lazy component,
      so we just have to sneak it into the DOM, which is kind of annoying, but means
      that users shouldn't ever spend time waiting for it to load.
      - Except we don't do it when running tests, because it would make them
      slower anyway. */}
			{process.env.NODE_ENV !== "test" && (
				<div className="sr-only" aria-hidden="true">
					<Suspense>
						<EmojiPicker onEmojiSelect={() => {}} />
					</Suspense>
				</div>
			)}
		</div>
	);
};
