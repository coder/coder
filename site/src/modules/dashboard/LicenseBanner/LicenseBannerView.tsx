import { cva } from "class-variance-authority";
import { ChevronRightIcon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Link } from "#/components/Link/Link";
import { cn } from "#/utils/cn";

const formatMessage = (message: string) => {
	// If the message ends with an alphanumeric character, add a period.
	if (/[a-z0-9]$/i.test(message)) {
		return `${message}.`;
	}
	return message;
};

type LicenseBannerVariant = "warning" | "warningProminent" | "error";

export interface LicenseBannerLink {
	href: string;
	label: string;
	showExternalIcon?: boolean;
	target?: React.ComponentProps<typeof Link>["target"];
}

export interface LicenseBannerMessage {
	message: string;
	variant: LicenseBannerVariant;
	// Diagnostics about the license or the usage measurement rather than
	// about usage itself. They keep the "License notices" heading even when
	// they are the only message, since the muted text needs that context.
	kind?: "diagnostic";
	link?: LicenseBannerLink;
}

const bannerVariants = cva("flex items-center p-3", {
	variants: {
		variant: {
			warning: "bg-surface-secondary",
			warningProminent: "bg-surface-orange",
			error: "bg-surface-red",
		},
	},
});

const iconVariants = cva("size-4", {
	variants: {
		variant: {
			warning: "text-content-warning",
			warningProminent: "text-content-warning",
			error: "text-content-destructive",
		},
	},
});

interface LicenseBannerViewProps {
	messages: readonly LicenseBannerMessage[];
}

const messageLinkClass = "text-xs font-medium text-content-link!";
const listClass =
	"m-0 list-disc space-y-1 pl-4 text-xs leading-[18px] text-content-primary";

const getBannerVariant = (
	messages: readonly LicenseBannerMessage[],
): LicenseBannerVariant => {
	const hasError = messages.some((entry) => entry.variant === "error");
	if (hasError) {
		return "error";
	}

	const hasProminentWarning = messages.some(
		(entry) => entry.variant === "warningProminent",
	);
	return hasProminentWarning ? "warningProminent" : "warning";
};

// The muted "warning" variant means every message is an advisory or
// diagnostic, so the heading must not assert a limit was hit. The prominent
// heading says "reached" rather than "exceeded" because some limit warnings
// fire at exact equality, which "reached" covers in both cases.
const bannerTitle = (variant: LicenseBannerVariant): string => {
	switch (variant) {
		case "error":
			return "License errors require attention";
		case "warningProminent":
			return "Your license limits have been reached";
		case "warning":
			return "License notices";
	}
};

const bannerRole = (variant: LicenseBannerVariant): "alert" | "status" =>
	variant === "error" ? "alert" : "status";

const LicenseMessageText: React.FC<{
	entry: LicenseBannerMessage;
}> = ({ entry }) => (
	<>
		{formatMessage(entry.message)}{" "}
		{entry.link && (
			<Link
				className={messageLinkClass}
				href={entry.link.href}
				showExternalIcon={entry.link.showExternalIcon}
				target={entry.link.target}
			>
				{entry.link.label}
			</Link>
		)}
	</>
);

const LicenseMessageList: React.FC<{
	messages: readonly LicenseBannerMessage[];
}> = ({ messages }) => (
	<ul className={listClass}>
		{messages.map((entry, index) => (
			<li key={`${entry.message}-${index}`}>
				<LicenseMessageText entry={entry} />
			</li>
		))}
	</ul>
);

const ExpandableLicenseMessageList: React.FC<{
	visibleMessages: readonly LicenseBannerMessage[];
	hiddenMessages: readonly LicenseBannerMessage[];
}> = ({ visibleMessages, hiddenMessages }) => {
	const [showDetails, setShowDetails] = useState(false);

	return (
		<div className="flex flex-col gap-1">
			<LicenseMessageList messages={visibleMessages} />
			{hiddenMessages.length > 0 && (
				<Collapsible open={showDetails} onOpenChange={setShowDetails}>
					<CollapsibleContent>
						<div className="text-content-primary text-xs">
							<LicenseMessageList messages={hiddenMessages} />
						</div>
					</CollapsibleContent>
					{/* asChild: the trigger must not render its own <button>
					    around Button, which is invalid HTML and exposes two
					    identically named buttons to assistive tech. */}
					<CollapsibleTrigger asChild>
						<Button
							className="text-xs mt-0.5 text-content-primary px-0"
							variant="subtle"
							size="sm"
						>
							<ChevronRightIcon
								className={cn(
									"transition-transform duration-200",
									showDetails ? "-rotate-90" : "",
								)}
							/>
							<span>{showDetails ? "Show less" : "Show more"}</span>
						</Button>
					</CollapsibleTrigger>
				</Collapsible>
			)}
		</div>
	);
};

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({
	messages,
}) => {
	if (messages.length === 0) {
		return null;
	}

	const isSingleMessage = messages.length === 1;
	const bannerVariant = getBannerVariant(messages);
	const visibleMessages = messages.slice(0, 2);
	const hiddenMessages = messages.slice(2);
	// A lone diagnostic keeps the heading: without it the muted banner is an
	// unexplained sentence. Other single messages stay heading-less.
	const showHeading = !isSingleMessage || messages[0].kind === "diagnostic";

	return (
		<div
			role={bannerRole(bannerVariant)}
			className={cn(bannerVariants({ variant: bannerVariant }))}
		>
			<div className="flex min-w-0 flex-1 items-start gap-2">
				<div className="flex h-6 items-center">
					<TriangleAlertIcon
						className={cn(iconVariants({ variant: bannerVariant }))}
					/>
				</div>
				<div className="flex min-w-0 flex-1 flex-col gap-2">
					{showHeading && (
						<div className="text-sm font-semibold leading-6 text-content-primary">
							{bannerTitle(bannerVariant)}
						</div>
					)}
					{isSingleMessage ? (
						<div className="flex min-h-6 items-center text-xs leading-4 text-content-primary">
							<LicenseMessageText entry={messages[0]} />
						</div>
					) : (
						<ExpandableLicenseMessageList
							hiddenMessages={hiddenMessages}
							visibleMessages={visibleMessages}
						/>
					)}
				</div>
			</div>
		</div>
	);
};
