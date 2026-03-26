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

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({
	messages,
}) => {
	const [showDetails, setShowDetails] = useState(false);
	const hasError = messages.some((entry) => entry.variant === "error");
	const hasProminentWarning = messages.some(
		(entry) => entry.variant === "warningProminent",
	);
	const isSingleMessage = messages.length === 1;
	const bannerVariant: LicenseBannerVariant = hasError
		? "error"
		: hasProminentWarning
			? "warningProminent"
			: "warning";
	const showExpander = messages.length > 2;
	const visibleMessages = showExpander ? messages.slice(0, 2) : messages;
	const hiddenMessages = showExpander ? messages.slice(2) : [];

	return (
		<div
			role="alert"
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
							{formatMessage(messages[0].message)}{" "}
							{messages[0].link && (
								<Link
									className="text-xs font-medium !text-content-link"
									href={messages[0].link.href}
									showExternalIcon={messages[0].link.showExternalIcon}
									target={messages[0].link.target}
								>
									{messages[0].link.label}
								</Link>
							)}
						</div>
					) : (
						<>
							<div className="text-sm font-semibold leading-6 text-content-primary">
								Your license limits have been exceeded
							</div>
							<div className="flex flex-col gap-1">
								<ul className="m-0 list-disc space-y-1 pl-4 text-xs leading-[18px] text-content-primary">
									{visibleMessages.map((entry, index) => (
										<li key={`${entry.message}-${index}`}>
											{formatMessage(entry.message)}{" "}
											{entry.link && (
												<Link
													className="text-xs font-medium !text-content-link"
													href={entry.link.href}
													showExternalIcon={entry.link.showExternalIcon}
													target={entry.link.target}
												>
													{entry.link.label}
												</Link>
											)}
										</li>
									))}
								</ul>
								{showExpander && (
									<Expander expanded={showDetails} setExpanded={setShowDetails}>
										<ul className="m-0 list-disc space-y-1 pl-4 text-xs leading-[18px] text-content-primary">
											{hiddenMessages.map((entry, index) => (
												<li key={`${entry.message}-${index}`}>
													{formatMessage(entry.message)}{" "}
													{entry.link && (
														<Link
															className="text-xs font-medium !text-content-link"
															href={entry.link.href}
															showExternalIcon={entry.link.showExternalIcon}
															target={entry.link.target}
														>
															{entry.link.label}
														</Link>
													)}
												</li>
											))}
										</ul>
									</Expander>
								)}
							</div>
						</>
					)}
				</div>
			</div>
		</div>
	);
};
