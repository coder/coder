import { cn } from "cn";
import { PlusIcon } from "lucide-react";
import { useId } from "react";
import { getErrorDetail, getErrorMessageWithValidations } from "#/api/errors";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";

type SectionErrorProps = {
	readonly error: unknown;
	readonly onRetry: () => void;
};

/** Shows a load failure with a Retry action. */
export const SectionError: React.FC<SectionErrorProps> = ({
	error,
	onRetry,
}) => (
	<ErrorAlert
		error={error}
		actions={
			<Button size="sm" variant="outline" onClick={onRetry}>
				Retry
			</Button>
		}
	/>
);

/** A card with one row-high skeleton, so loading does not shift the layout. */
const SectionSkeleton: React.FC = () => (
	<SettingsCard>
		<SettingsCardRow>
			<Skeleton variant="text" className="w-2/3" />
			<span className="sr-only">Loading</span>
		</SettingsCardRow>
	</SettingsCard>
);

/** Shows the load failure if there is one, otherwise the loading skeleton. */
export const SectionLoadState: React.FC<SectionErrorProps> = ({
	error,
	onRetry,
}) =>
	error ? (
		<SectionError error={error} onRetry={onRetry} />
	) : (
		<SectionSkeleton />
	);

type SectionSaveErrorProps = {
	readonly error: unknown;
	/** Shown when the error carries no message. */
	readonly fallback?: string;
};

/**
 * Shows why a change failed to save, with its field errors and detail, in a
 * section.
 */
export const SectionSaveError: React.FC<SectionSaveErrorProps> = ({
	error,
	fallback = "Failed to save changes.",
}) => {
	const message = getErrorMessageWithValidations(error, fallback);
	const detail = getErrorDetail(error);
	return (
		<Alert severity="error" prominent>
			<AlertDescription>
				{detail && !message.includes(detail) ? `${message} ${detail}` : message}
			</AlertDescription>
		</Alert>
	);
};

type SettingsGroupProps = {
	/** A short name for what the sections below have in common. */
	readonly title: string;
	/** One line under the title. */
	readonly description?: string;
	readonly children: React.ReactNode;
};

/**
 * A titled run of `SettingsSection`s that belong together, such as everything
 * an agent can use. The page is a stack of groups.
 */
export const SettingsGroup: React.FC<SettingsGroupProps> = ({
	title,
	description,
	children,
}) => (
	<div className="flex flex-col gap-6 border-0 border-t border-solid border-border pt-8 first:border-t-0 first:pt-0">
		<div className="flex flex-col gap-0.5">
			<h2 className="m-0 text-lg font-semibold text-content-primary">
				{title}
			</h2>
			{description && (
				<p className="m-0 text-sm text-content-secondary">{description}</p>
			)}
		</div>
		{children}
	</div>
);

type SettingsSectionProps = {
	/** A short label such as "Triggers". */
	readonly title: string;
	/** One line under the label saying what the section controls. */
	readonly description?: string;
	/** Controls beside the label, such as a Save button. */
	readonly actions?: React.ReactNode;
	/** Lets other places link to the section with `#id`. */
	readonly id?: string;
	/** The heading level. Use 2 for a section that is not inside a group. */
	readonly headingLevel?: 2 | 3;
	readonly children: React.ReactNode;
};

/** A labeled block of the agent settings. Put a `SettingsCard` inside. */
export const SettingsSection: React.FC<SettingsSectionProps> = ({
	title,
	description,
	actions,
	id,
	headingLevel = 3,
	children,
}) => {
	const headingId = useId();
	const Heading = headingLevel === 2 ? "h2" : "h3";
	return (
		<section
			id={id}
			aria-labelledby={headingId}
			className="flex flex-col gap-3"
		>
			<div className="flex min-h-8 flex-wrap items-center justify-between gap-x-4 gap-y-2">
				<div className="flex min-w-0 flex-1 flex-col gap-0.5">
					<Heading
						id={headingId}
						className="m-0 text-sm font-semibold text-content-primary"
					>
						{title}
					</Heading>
					{description && (
						<p className="m-0 text-xs leading-5 text-content-secondary">
							{description}
						</p>
					)}
				</div>
				{actions}
			</div>
			{children}
		</section>
	);
};

const cardClassName =
	"m-0 flex list-none flex-col divide-y divide-border overflow-hidden rounded-lg border border-solid border-border bg-surface-primary p-0";

type SettingsCardProps =
	| ({
			/** The default. */
			readonly as?: "ul";
	  } & React.ComponentProps<"ul">)
	| ({
			/** Use `div` when the rows are not a list, such as a single text field. */
			readonly as: "div";
	  } & React.ComponentProps<"div">);

/** A rounded, bordered card whose rows are separated by dividers. */
export const SettingsCard: React.FC<SettingsCardProps> = (props) => {
	if (props.as === "div") {
		const { as: _as, className, ...divProps } = props;
		return <div className={cn(cardClassName, className)} {...divProps} />;
	}
	const { as: _as, className, ...ulProps } = props;
	return <ul className={cn(cardClassName, className)} {...ulProps} />;
};

const rowClassName =
	"flex min-h-12 min-w-0 items-center gap-3 px-4 py-3 text-sm";

type SettingsCardRowProps =
	| ({
			/** The default. */
			readonly as?: "li";
	  } & React.ComponentProps<"li">)
	| ({
			/** Use `div` inside a `SettingsCard` that renders as a `div`. */
			readonly as: "div";
	  } & React.ComponentProps<"div">);

/** One row of a `SettingsCard`. Lay out its content with flex utilities. */
export const SettingsCardRow: React.FC<SettingsCardRowProps> = (props) => {
	if (props.as === "div") {
		const { as: _as, className, ...divProps } = props;
		return <div className={cn(rowClassName, className)} {...divProps} />;
	}
	const { as: _as, className, ...liProps } = props;
	return <li className={cn(rowClassName, className)} {...liProps} />;
};

type SettingsCardRowTextProps = {
	/** Pass a link to make the title navigable. */
	readonly title: React.ReactNode;
	/** A line of helper text under the title. */
	readonly description?: React.ReactNode;
};

/** The title and helper text of a row, growing to push controls to the right. */
export const SettingsCardRowText: React.FC<SettingsCardRowTextProps> = ({
	title,
	description,
}) => (
	<div className="flex min-w-0 flex-1 flex-col">
		<span className="truncate text-content-primary">{title}</span>
		{description && (
			<span className="wrap-anywhere text-xs text-content-secondary">
				{description}
			</span>
		)}
	</div>
);

type SettingsCardRowIconProps = {
	readonly className?: string;
	readonly children: React.ReactNode;
};

/**
 * The box for the icon or logo that starts a row, so row text lines up
 * across sections. It sizes line icons and logos alike.
 */
export const SettingsCardRowIcon: React.FC<SettingsCardRowIconProps> = ({
	className,
	children,
}) => (
	<span
		className={cn(
			"flex size-6 shrink-0 items-center justify-center [&>img]:size-5 [&>img]:object-contain [&>svg]:size-icon-sm",
			className,
		)}
	>
		{children}
	</span>
);

type SettingsCardEmptyRowProps = {
	readonly children: React.ReactNode;
};

/**
 * The single muted row of a card with nothing in it. Say "No X yet." for a
 * list people add to, and "No X are configured." for a list set up elsewhere.
 */
export const SettingsCardEmptyRow: React.FC<SettingsCardEmptyRowProps> = ({
	children,
}) => (
	<SettingsCardRow className="text-content-secondary">
		{children}
	</SettingsCardRow>
);

type SettingsCardAddRowProps = Omit<
	React.ComponentProps<"button">,
	"children"
> & {
	/** For example "Add agent". A plus icon precedes it. */
	readonly label: string;
};

/**
 * The muted "+ Add ..." row that ends a `SettingsCard`. It spreads its props
 * onto the button, so it works as a dropdown or popover trigger.
 */
export const SettingsCardAddRow: React.FC<SettingsCardAddRowProps> = ({
	label,
	className,
	...props
}) => (
	<li className="flex">
		<button
			type="button"
			className={cn(
				"flex min-h-12 w-full cursor-pointer items-center gap-2 border-0 bg-transparent px-4 py-3 text-left text-sm text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:opacity-50",
				className,
			)}
			{...props}
		>
			<PlusIcon aria-hidden className="size-icon-sm" />
			{label}
		</button>
	</li>
);
