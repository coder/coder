import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import type { Workspace } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { useProxy } from "#/contexts/ProxyContext";
import { useAppLink } from "#/modules/apps/useAppLink";
import type { MuxAppCandidate } from "./muxApps";

type MuxIframeProps = {
	workspace: Workspace;
	candidate: MuxAppCandidate;
};

export const MuxIframe: FC<MuxIframeProps> = ({ workspace, candidate }) => {
	const { agent, app } = candidate;
	const link = useAppLink(app, { agent, workspace });
	const proxy = useProxy();
	const workspacePath = `/@${workspace.owner_name}/${workspace.name}`;
	const shouldDisplayWildcardWarning =
		app.subdomain && !proxy.proxy?.preferredWildcardHostname;

	return (
		<div className="flex h-full min-h-0 flex-col bg-surface-primary">
			<div className="flex flex-col gap-3 border-0 border-b border-solid border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="min-w-0">
					<p className="m-0 truncate text-sm font-medium text-content-primary">
						{workspace.owner_name}/{workspace.name}
					</p>
					<p className="m-0 truncate text-xs text-content-secondary">
						{app.display_name ?? app.slug} on agent {agent.name}
					</p>
				</div>
				<Link
					href={link.href}
					target="_blank"
					rel="noreferrer"
					onClick={link.onClick}
					className="w-fit"
				>
					Open in new tab
				</Link>
			</div>

			<div className="min-h-0 flex-1">
				{workspace.latest_build.status === "stopped" ? (
					<StoppedWorkspace workspacePath={workspacePath} />
				) : shouldDisplayWildcardWarning ? (
					<MissingWildcardHostnameWarning />
				) : app.health === "healthy" || app.health === "disabled" ? (
					<MuxAppFrame src={link.href} title={link.label} />
				) : app.health === "initializing" ? (
					<InitializingMuxApp />
				) : app.health === "unhealthy" ? (
					<UnhealthyMuxApp
						workspacePath={workspacePath}
						healthcheckUrl={app.healthcheck?.url}
					/>
				) : (
					<UnknownHealthState />
				)}
			</div>
		</div>
	);
};

type MuxAppFrameProps = {
	src: string;
	title: string;
};

const MuxAppFrame: FC<MuxAppFrameProps> = ({ src, title }) => {
	return (
		<iframe
			data-testid="mux-iframe"
			src={src}
			title={title}
			loading="eager"
			allow="clipboard-read; clipboard-write"
			className="h-full w-full border-0"
		/>
	);
};

type StoppedWorkspaceProps = {
	workspacePath: string;
};

const StoppedWorkspace: FC<StoppedWorkspaceProps> = ({ workspacePath }) => {
	return (
		<div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center">
			<div>
				<h2 className="m-0 text-xl font-medium text-content-primary">
					Start this workspace to use Mux
				</h2>
				<p className="m-0 mt-2 max-w-md text-sm text-content-secondary">
					Mux is available in this workspace, but the workspace is stopped.
					Start it from the workspace page before opening the embedded app.
				</p>
			</div>
			<div className="flex flex-wrap items-center justify-center gap-3">
				<Button asChild>
					<RouterLink to={workspacePath}>Start workspace</RouterLink>
				</Button>
				<Link href={workspacePath} showExternalIcon={false}>
					Open workspace
				</Link>
			</div>
		</div>
	);
};

const MissingWildcardHostnameWarning: FC = () => {
	return (
		<div className="flex h-full items-center justify-center p-6">
			<Alert severity="warning" prominent className="max-w-2xl">
				<AlertTitle>
					Workspace app wildcard hostname is not configured
				</AlertTitle>
				<AlertDescription>
					This Mux app uses subdomains, but this deployment does not have a
					preferred wildcard hostname configured. Configure a wildcard hostname
					for workspace apps, then refresh this page.
				</AlertDescription>
			</Alert>
		</div>
	);
};

const InitializingMuxApp: FC = () => {
	return (
		<div
			role="status"
			aria-live="polite"
			className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center"
		>
			<Spinner loading />
			<div>
				<h2 className="m-0 text-xl font-medium text-content-primary">
					Mux is starting
				</h2>
				<p className="m-0 mt-2 text-sm text-content-secondary">
					Waiting for the Mux app healthcheck to pass.
				</p>
			</div>
		</div>
	);
};

type UnhealthyMuxAppProps = {
	workspacePath: string;
	healthcheckUrl?: string;
};

const UnhealthyMuxApp: FC<UnhealthyMuxAppProps> = ({
	workspacePath,
	healthcheckUrl,
}) => {
	return (
		<div className="flex h-full items-center justify-center p-6">
			<Alert
				severity="error"
				prominent
				className="max-w-2xl"
				actions={
					<Button asChild size="sm" variant="outline">
						<RouterLink to={workspacePath}>
							<ExternalLinkIcon aria-hidden="true" />
							Open workspace logs
						</RouterLink>
					</Button>
				}
			>
				<AlertTitle>Mux app healthcheck is failing</AlertTitle>
				<AlertDescription>
					The Mux app is reporting an unhealthy state. Check the workspace logs
					and verify the Mux app healthcheck.
					{healthcheckUrl ? (
						<span className="mt-2 block">
							Healthcheck URL:{" "}
							<code className="select-all rounded-sm bg-surface-tertiary px-1 py-0.5 font-mono text-content-primary">
								{healthcheckUrl}
							</code>
							.
						</span>
					) : null}
				</AlertDescription>
			</Alert>
		</div>
	);
};

const UnknownHealthState: FC = () => {
	return (
		<div className="flex h-full items-center justify-center p-6">
			<Alert severity="error" prominent className="max-w-2xl">
				<AlertTitle>Mux app health state is unknown</AlertTitle>
				<AlertDescription>
					Refresh this page or open the app in a new tab after the workspace
					finishes reporting app health.
				</AlertDescription>
			</Alert>
		</div>
	);
};
