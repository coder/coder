import type { FC } from "react";
import {
	Alert,
	AlertDescription,
	AlertTitle,
} from "#/components/Alert/Alert";

interface BuildErrorAlertProps {
	error: string;
	title?: string;
}

// Terraform prefixes error messages with this boilerplate. Strip it so
// users see only the actionable message from the template author.
const terraformBoilerplatePrefix =
	"terraform plan: exit status 1\n\nCoder encountered an error with your template, which may be related to an issue with the template's Terraform code, or a problem with the provisioner. Check the plan logs for more details.\n\nDiagnostics:\n";

// Match URLs so we can render them as clickable links. Non-global so we
// can use it with string.split() which needs a non-global regex to
// produce the correct alternating text/url segments.
const urlPattern = /(https?:\/\/[^\s)]+)/;

/**
 * Renders a build error with multi-line formatting and clickable URLs.
 * Strips terraform boilerplate so template authors can write clear,
 * actionable error messages in preconditions.
 */
export const BuildErrorAlert: FC<BuildErrorAlertProps> = ({
	error,
	title = "Workspace build failed",
}) => {
	let message = error;
	if (message.startsWith(terraformBoilerplatePrefix)) {
		message = message.slice(terraformBoilerplatePrefix.length);
	}

	// Split on real newlines and escaped newlines (from JSON encoding).
	// Group into paragraphs separated by blank lines so the rendered
	// output preserves the visual structure the template author intended.
	const lines = message.split(/\n|\\n/);
	const paragraphs: string[][] = [];
	let current: string[] = [];
	for (const line of lines) {
		const trimmed = line.trim();
		if (trimmed.length === 0) {
			if (current.length > 0) {
				paragraphs.push(current);
				current = [];
			}
		} else {
			current.push(trimmed);
		}
	}
	if (current.length > 0) {
		paragraphs.push(current);
	}

	return (
		<Alert severity="error" prominent>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>
				<div className="flex flex-col gap-3">
					{paragraphs.map((para, pi) => (
						<div key={pi} className="flex flex-col gap-1">
							{para.map((line, li) => (
								<p key={li} className="m-0">
									{linkify(line)}
								</p>
							))}
						</div>
					))}
				</div>
			</AlertDescription>
		</Alert>
	);
};

/**
 * Replace URLs in a line with anchor elements.
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
