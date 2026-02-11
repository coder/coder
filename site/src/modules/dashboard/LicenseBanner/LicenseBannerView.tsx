import { LicenseTelemetryRequiredErrorText } from "api/typesGenerated";
import { Expander } from "components/Expander/Expander";
import { Pill } from "components/Pill/Pill";
import { type FC, useState } from "react";
import { cn } from "utils/cn";

const formatMessage = (message: string) => {
	// If the message ends with an alphanumeric character, add a period.
	if (/[a-z0-9]$/i.test(message)) {
		return `${message}.`;
	}
	return message;
};

interface LicenseBannerViewProps {
	errors: readonly string[];
	warnings: readonly string[];
}

export const LicenseBannerView: FC<LicenseBannerViewProps> = ({
	errors,
	warnings,
}) => {
	const [showDetails, setShowDetails] = useState(false);
	const isError = errors.length > 0;
	const messages = [...errors, ...warnings];
	const type = isError ? "error" : "warning";

	if (messages.length === 1) {
		const [message] = messages;

		return (
			<div
				className={cn(
					"flex items-center p-3 text-sm",
					isError ? "bg-surface-red" : "bg-surface-orange",
				)}
			>
				<Pill type={type}>License Issue</Pill>
				<div className="mx-2">
					<span>{formatMessage(messages[0])}</span>
					&nbsp;
					<a
						className={cn(
							"font-medium underline",
							isError ? "text-content-destructive" : "text-content-warning",
						)}
						href="mailto:sales@coder.com"
					>
						{message === LicenseTelemetryRequiredErrorText
							? "Contact sales@coder.com if you need an exception."
							: "Contact sales@coder.com."}
					</a>
				</div>
			</div>
		);
	}

	return (
		<div
			className={cn(
				"flex items-center p-3 text-sm",
				isError ? "bg-surface-red" : "bg-surface-orange",
			)}
		>
			<Pill type={type}>{`${messages.length} License Issues`}</Pill>
			<div className="mx-2">
				<div>
					It looks like you've exceeded some limits of your license. &nbsp;
					<a
						className={cn(
							"font-medium underline",
							isError ? "text-content-destructive" : "text-content-warning",
						)}
						href="mailto:sales@coder.com"
					>
						Contact sales@coder.com.
					</a>
				</div>
				<Expander expanded={showDetails} setExpanded={setShowDetails}>
					<ul className="p-2 m-0">
						{messages.map((message) => (
							<li className="m-1" key={message}>
								{formatMessage(message)}
							</li>
						))}
					</ul>
				</Expander>
			</div>
		</div>
	);
};
