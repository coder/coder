import { cva } from "class-variance-authority";
import { TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { Expander } from "#/components/Expander/Expander";
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

const messageLinkClass = "text-xs font-medium !text-content-link";
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

const bannerTitle = (variant: LicenseBannerVariant): string =>
	variant === "error"
		? "License errors require attention"
		: "Your license limits have been exceeded";

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
	const showExpander = hiddenMessages.length > 0;

	return (
		<div className="flex flex-col gap-1">
			<LicenseMessageList messages={visibleMessages} />
			{showExpander && (
				<Expander expanded={showDetails} setExpanded={setShowDetails}>
					<LicenseMessageList messages={hiddenMessages} />
				</Expander>
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
					{isSingleMessage ? (
						<div className="flex min-h-6 items-center text-xs leading-4 text-content-primary">
							<LicenseMessageText entry={messages[0]} />
						</div>
					) : (
						<>
							<div className="text-sm font-semibold leading-6 text-content-primary">
								{bannerTitle(bannerVariant)}
							</div>
							<ExpandableLicenseMessageList
								hiddenMessages={hiddenMessages}
								visibleMessages={visibleMessages}
							/>
						</>
					)}
				</div>
			</div>
		</div>
	);
};
