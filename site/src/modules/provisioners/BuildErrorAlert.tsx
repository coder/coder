import type { FC } from "react";
import type { ProvisionerJobDiagnostic } from "#/api/typesGenerated";
import {
	Alert,
	AlertDescription,
	AlertTitle,
} from "#/components/Alert/Alert";

interface BuildErrorAlertProps {
	error: string;
	diagnostics?: readonly ProvisionerJobDiagnostic[];
	title?: string;
}

// Match URLs so we can render them as clickable links. Non-global so we
// can use it with string.split() which needs a non-global regex to
// produce the correct alternating text/url segments.
const urlPattern = /(https?:\/\/[^\s)]+)/;

/**
 * Renders a build error with multi-line formatting and clickable URLs.
 * When structured diagnostics are available, renders them directly.
 * Falls back to string parsing for older API responses.
 */
export const BuildErrorAlert: FC<BuildErrorAlertProps> = ({
	error,
	diagnostics,
	title = "Workspace build failed",
}) => {
	// Prefer structured diagnostics when available.
	if (diagnostics && diagnostics.length > 0) {
		return (
			<Alert severity="error" prominent>
				<AlertTitle>{title}</AlertTitle>
				<AlertDescription>
					<div className="flex flex-col gap-3">
						{diagnostics.map((diag, i) => (
							<div key={i} className="flex flex-col gap-1">
								<p className="m-0 font-medium">
									{linkify(diag.summary)}
								</p>
								{diag.detail && (
									<div className="flex flex-col gap-1">
										{splitIntoParagraphs(diag.detail).map(
											(para, pi) => (
												<p key={pi} className="m-0">
													{linkify(para)}
												</p>
											),
										)}
									</div>
								)}
							</div>
						))}
					</div>
				</AlertDescription>
			</Alert>
		);
	}

	// Fallback: parse the error string for older API responses that
	// don't include structured diagnostics.
	const paragraphs = splitIntoParagraphs(error);
	return (
		<Alert severity="error" prominent>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>
				<div className="flex flex-col gap-3">
					{paragraphs.map((para, i) => (
						<p key={i} className="m-0">
							{linkify(para)}
						</p>
					))}
				</div>
			</AlertDescription>
		</Alert>
	);
};

/**
 * Split text into paragraphs on blank lines, trimming each line.
 */
function splitIntoParagraphs(text: string): string[] {
	const lines = text.split(/\n|\\n/);
	const paragraphs: string[] = [];
	let current: string[] = [];
	for (const line of lines) {
		const trimmed = line.trim();
		if (trimmed.length === 0) {
			if (current.length > 0) {
				paragraphs.push(current.join(" "));
				current = [];
			}
		} else {
			current.push(trimmed);
		}
	}
	if (current.length > 0) {
		paragraphs.push(current.join(" "));
	}
	return paragraphs;
}

/**
 * Replace URLs in text with anchor elements.
 */
function linkify(text: string): React.ReactNode {
	const parts = text.split(urlPattern);
	if (parts.length === 1) {
		return text;
	}
	return parts.map((part, i) => {
		if (urlPattern.test(part)) {
			return (
				<a
					key={i}
					href={part}
					target="_blank"
					rel="noopener noreferrer"
					className="text-content-link underline"
				>
					{part}
				</a>
			);
		}
		return part;
	});
}
