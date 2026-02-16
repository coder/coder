import {
	LicenseManagedAgentLimitExceededWarningText,
	LicenseTelemetryRequiredErrorText,
} from "api/typesGenerated";
import { Expander } from "components/Expander/Expander";
import { Link } from "components/Link/Link";
import { Pill } from "components/Pill/Pill";
import { useState } from "react";
import { cn } from "utils/cn";
import { docs } from "utils/docs";

const formatMessage = (message: string) => {
	// If the message ends with an alphanumeric character, add a period.
	if (/[a-z0-9]$/i.test(message)) {
		return `${message}.`;
	}
	return message;
};

const messageLinkProps = (
	message: string,
): Pick<React.ComponentProps<typeof Link>, "href" | "children" | "target"> => {
	if (message === LicenseManagedAgentLimitExceededWarningText) {
		return {
			href: docs("/ai-coder/ai-governance"),
			children: "View AI Governance",
			target: "_blank",
		};
	}
	if (message === LicenseTelemetryRequiredErrorText) {
		return {
			href: "mailto:sales@coder.com",
			children: "Contact sales@coder.com if you need an exception.",
		};
	}
	return {
		href: "mailto:sales@coder.com",
		children: "Contact sales@coder.com.",
	};
};

interface LicenseBannerViewProps {
	errors: readonly string[];
	warnings: readonly string[];
}

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({
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
					<span>{formatMessage(message)}</span>
					&nbsp;
					<Link
						className={cn(
							"font-medium",
							isError ? "!text-content-destructive" : "!text-content-warning",
						)}
						{...messageLinkProps(message)}
					/>
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
				<div>It looks like you've exceeded some limits of your license.</div>
				<Expander expanded={showDetails} setExpanded={setShowDetails}>
					<ul className="p-2 m-0">
						{messages.map((message) => (
							<li className="m-1" key={message}>
								{formatMessage(message)}&nbsp;
								<Link
									className={cn(
										"font-medium text-xs px-0",
										isError
											? "!text-content-destructive"
											: "!text-content-warning",
									)}
									{...messageLinkProps(message)}
								/>
							</li>
						))}
					</ul>
				</Expander>
			</div>
		</div>
	);
};
