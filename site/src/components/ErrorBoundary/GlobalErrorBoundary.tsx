/**
 * @file A global error boundary designed to work with React Router.
 *
 * This is not documented well, but because of React Router works, it will
 * automatically intercept any render errors produced in routes, and will
 * "swallow" them, preventing the errors from bubbling up to any error
 * boundaries above the router. The global error boundary must be explicitly
 * bound to a route to work as expected.
 */
import type { Interpolation } from "@emotion/react";
import Link from "@mui/material/Link";
import { Button } from "components/Button/Button";
import { CoderIcon } from "components/Icons/CoderIcon";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import {
	type ErrorResponse,
	isRouteErrorResponse,
	useLocation,
	useRouteError,
} from "react-router-dom";

const errorPageTitle = "Something went wrong";

// Mocking React Router's error-handling logic is a pain; the next best thing is
// to split it off from the rest of the code, and pass the value via props
export const GlobalErrorBoundary: FC = () => {
	const error = useRouteError();
	return <GlobalErrorBoundaryInner error={error} />;
};

type GlobalErrorBoundaryInnerProps = Readonly<{ error: unknown }>;
export const GlobalErrorBoundaryInner: FC<GlobalErrorBoundaryInnerProps> = ({
	error,
}) => {
	const [showErrorMessage, setShowErrorMessage] = useState(false);
	const { metadata } = useEmbeddedMetadata();
	const location = useLocation();

	const coderVersion = metadata["build-info"].value?.version;
	const isRenderableError =
		error instanceof Error || isRouteErrorResponse(error);

	return (
		<div className="bg-surface-primary text-center w-full h-full flex justify-center items-center">
			<Helmet>
				<title>{errorPageTitle}</title>
			</Helmet>

			<main className="flex gap-6 w-full max-w-prose p-4 flex-col flex-nowrap">
				<div className="flex gap-2 flex-col items-center">
					<CoderIcon className="w-11 h-11" />

					<div className="text-content-primary flex flex-col gap-1">
						<h1 className="text-2xl font-normal m-0">{errorPageTitle}</h1>
						<p className="leading-6 m-0">
							Please try reloading the page. If reloading does not work, you can
							ask for help in the{" "}
							<Link
								href="https://discord.gg/coder"
								target="_blank"
								rel="noreferer"
							>
								Coder Discord community
								<span className="sr-only"> (link opens in a new tab)</span>
							</Link>{" "}
							or{" "}
							<Link
								target="_blank"
								rel="noreferer"
								href={publicGithubIssueLink(
									coderVersion,
									location.pathname,
									error,
								)}
							>
								open an issue on GitHub
								<span className="sr-only"> (link opens in a new tab)</span>
							</Link>
							.
						</p>
					</div>
				</div>

				<div className="flex flex-row flex-nowrap justify-center gap-4">
					<Button asChild className="min-w-32 font-medium">
						<Link href={location.pathname}>Reload page</Link>
					</Button>

					{isRenderableError && (
						<Button
							variant="outline"
							className="min-w-32"
							onClick={() => setShowErrorMessage(!showErrorMessage)}
						>
							{showErrorMessage ? "Hide error" : "Show error"}
						</Button>
					)}
				</div>

				{isRenderableError && showErrorMessage && <ErrorStack error={error} />}
			</main>
		</div>
	);
};

type ErrorStackProps = Readonly<{ error: Error | ErrorResponse }>;
const ErrorStack: FC<ErrorStackProps> = ({ error }) => {
	return (
		<aside className="p-4 text-left rounded-md border-[1px] border-content-tertiary border-solid">
			{isRouteErrorResponse(error) ? (
				<>
					<h2 className="text-base font-bold text-content-primary m-0">
						HTTP {error.status} - {error.statusText}
					</h2>
					<pre className="m-0 py-2 px-0 overflow-x-auto text-xs">
						<code data-testid="code">{serializeDataAsJson(error.data)}</code>
					</pre>
				</>
			) : (
				<>
					<h2 className="text-base font-bold text-content-primary m-0">
						{error.name}
					</h2>
					<p data-testid="description" className="pb-4 leading-5 m-0">
						{error.message}
					</p>
					{error.stack && (
						<pre className="m-0 py-2 px-0 overflow-x-auto text-xs">
							<code data-testid="code" data-chromatic="ignore">
								{error.stack}
							</code>
						</pre>
					)}
				</>
			)}
		</aside>
	);
};

function serializeDataAsJson(data: unknown): string | null {
	try {
		return JSON.stringify(data, null, 2);
	} catch {
		return null;
	}
}

function publicGithubIssueLink(
	coderVersion: string | undefined,
	pathName: string,
	error: unknown,
): string {
	const baseLink = "https://github.com/coder/coder/issues/new";

	// Anytime you see \`\`\`txt, that's wrapping the text in a GitHub codeblock
	let printableError: string;
	if (error instanceof Error) {
		printableError = [
			`${error.name}: ${error.message}`,
			error.stack ? `\`\`\`txt\n${error.stack}\n\`\`\`` : "No stack",
		].join("\n");
	} else if (isRouteErrorResponse(error)) {
		const serialized = serializeDataAsJson(error.data);
		printableError = [
			`HTTP ${error.status} - ${error.statusText}`,
			serialized ? `\`\`\`txt\n${serialized}\n\`\`\`` : "(No data)",
		].join("\n");
	} else {
		printableError = "No error message available";
	}

	const messageBody = `\
**Version**
${coderVersion ?? "-- Set version --"}

**Path**
\`${pathName}\`

**Error**
${printableError}`;

	return `${baseLink}?body=${encodeURIComponent(messageBody)}`;
}
