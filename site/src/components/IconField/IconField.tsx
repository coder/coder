import {
	type ComponentPropsWithRef,
	type FC,
	lazy,
	type ReactNode,
	Suspense,
	useId,
	useState,
} from "react";
import { ChevronDownIcon as AnimatedChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

const EmojiPicker = lazy(() => import("./EmojiPicker"));

type IconFieldProps = Omit<ComponentPropsWithRef<"input">, "type"> & {
	label?: ReactNode;
	error?: boolean;
	helperText?: ReactNode;
	onPickEmoji: (value: string) => void;
	/** Accepted for call-site compatibility with former MUI TextField usage. */
	fullWidth?: boolean;
};

export const IconField: FC<IconFieldProps> = ({
	id: idProp,
	value,
	label = "Icon",
	error,
	helperText,
	disabled,
	className,
	onPickEmoji,
	fullWidth: _fullWidth,
	...inputProps
}) => {
	if (typeof value !== "string" && typeof value !== "undefined") {
		throw new Error(`Invalid icon value "${typeof value}"`);
	}

	const generatedId = useId();
	const id = idProp ?? generatedId;
	const errorId = `${id}-error`;
	const helperId = `${id}-helper`;
	const [open, setOpen] = useState(false);
	const stringValue = value ?? "";
	const hasIcon = stringValue !== "";

	return (
		<div className="flex w-full flex-col gap-2">
			{label ? (
				<Label htmlFor={id} className="text-sm">
					{label}
				</Label>
			) : null}
			<InputGroup>
				<InputGroupInput
					{...inputProps}
					id={id}
					value={stringValue}
					disabled={disabled}
					aria-invalid={error}
					aria-describedby={
						helperText ? (error ? errorId : helperId) : undefined
					}
					className={cn("min-w-0 placeholder:text-content-disabled", className)}
					spellCheck={false}
				/>
				<InputGroupAddon align="inline-end" className="gap-1.5">
					{hasIcon && (
						<span className="flex size-5 items-center justify-center">
							<ExternalImage
								alt=""
								src={stringValue}
								className="max-w-full object-contain"
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
										onPickEmoji(picked);
										setOpen(false);
									}}
								/>
							</Suspense>
						</PopoverContent>
					</Popover>
				</InputGroupAddon>
			</InputGroup>
			{helperText ? (
				<span
					id={error ? errorId : helperId}
					className={cn(
						"text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</span>
			) : null}

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
