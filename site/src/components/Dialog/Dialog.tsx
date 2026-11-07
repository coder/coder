import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Button } from "#/components/Button/Button";
import {
	DialogPopup,
	type DialogPopupProps,
} from "#/components/Dialog/DialogPopup";
import { Spinner } from "#/components/Spinner/Spinner";

export const Dialog = DialogPrimitive.Root;

export const DialogTrigger = DialogPrimitive.Trigger;

const DialogPortal = DialogPrimitive.Portal;

export const DialogClose = DialogPrimitive.Close;

type DialogOverlayProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Backdrop>,
	"className"
> & {
	className?: string;
};

const DialogOverlay: React.FC<DialogOverlayProps> = ({
	className,
	...props
}) => {
	return (
		<DialogPrimitive.Backdrop
			className={cn(
				`fixed inset-0 z-50 bg-overlay ease-out-strong
			data-open:animate-in data-closed:animate-out
			data-closed:fade-out-0 data-open:fade-in-0`,
				className,
			)}
			{...props}
		/>
	);
};

const dialogVariants = cva(
	`fixed left-[50%] top-[50%] z-50 grid max-h-[90vh] w-full max-w-lg gap-6 overflow-y-auto
	border border-solid bg-surface-primary p-8 shadow-lg duration-200 ease-out-strong sm:rounded-lg
	-translate-1/2 outline-hidden
	data-open:animate-in data-closed:animate-out
	data-closed:fade-out-0 data-open:fade-in-0
	data-closed:zoom-out-95 data-open:zoom-in-95`,
	{
		variants: {
			variant: {
				default: "border-border",
				destructive: "border-border-destructive",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	},
);

type DialogContentProps = DialogPopupProps &
	VariantProps<typeof dialogVariants>;

export const DialogContent: React.FC<DialogContentProps> = ({
	className,
	variant,
	children,
	...props
}) => {
	return (
		<DialogPortal>
			<DialogOverlay />
			<DialogPopup
				className={cn(dialogVariants({ variant }), className)}
				{...props}
			>
				{children}
			</DialogPopup>
		</DialogPortal>
	);
};

export const DialogHeader: React.FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				"flex flex-col space-y-5 text-center sm:text-left",
				className,
			)}
			{...props}
		/>
	);
};

export const DialogFooter: React.FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			className={cn(
				"flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2",
				className,
			)}
			{...props}
		/>
	);
};

type DialogActionsProps = {
	/** Text to display in the confirm button */
	confirmText?: React.ReactNode;
	/** Whether or not confirm is loading, also disables cancel when true */
	confirmLoading?: boolean;
	/** Whether or not the submit button is disabled */
	confirmDisabled?: boolean;
	/** Whether the confirm button triggers a destructive action or not */
	confirmVariant?: React.ComponentProps<typeof Button>["variant"];
	/** Called when confirm is clicked */
	onConfirm?: () => void;

	/** Text to display in the cancel button */
	cancelText?: string;
	/** Called when cancel is clicked */
	onCancel?: () => void;
};

/**
 * Quickly handles most modals actions, some combination of a cancel and confirm button
 */
export const DialogActions: React.FC<DialogActionsProps> = ({
	confirmText = "Confirm",
	confirmLoading = false,
	confirmDisabled = false,
	confirmVariant,
	onConfirm,

	cancelText = "Cancel",
	onCancel,
}) => {
	return (
		<>
			{onCancel && (
				<Button
					disabled={confirmLoading}
					onClick={(e) => {
						e.stopPropagation();
						onCancel();
					}}
					variant="outline"
				>
					{cancelText}
				</Button>
			)}

			{onConfirm && (
				<Button
					variant={confirmVariant}
					disabled={confirmLoading || confirmDisabled}
					onClick={onConfirm}
					data-testid="confirm-button"
					type="submit"
				>
					<Spinner loading={confirmLoading} />
					{confirmText}
				</Button>
			)}
		</>
	);
};

type DialogTitleProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Title>,
	"className"
> & {
	className?: string;
};

export const DialogTitle: React.FC<DialogTitleProps> = ({
	className,
	...props
}) => {
	return (
		<DialogPrimitive.Title
			className={cn(
				"text-xl m-0 text-content-primary font-semibold leading-none tracking-tight",
				className,
			)}
			{...props}
		/>
	);
};

type DialogDescriptionProps = Omit<
	React.ComponentProps<typeof DialogPrimitive.Description>,
	"className"
> & { className?: string };

export const DialogDescription: React.FC<DialogDescriptionProps> = ({
	className,
	...props
}) => {
	return (
		<DialogPrimitive.Description
			className={cn("text-sm text-content-secondary font-medium", className)}
			{...props}
		/>
	);
};
