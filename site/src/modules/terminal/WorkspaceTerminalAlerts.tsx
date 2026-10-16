import { cn } from "cn";
import { RefreshCwIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import {
	Alert,
	type AlertColor,
	type AlertProps,
} from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";
import type { ConnectionStatus } from "./types";

type WorkspaceTerminalAlertsProps = {
	agent: WorkspaceAgent | undefined;
	status: ConnectionStatus;
	onAlertChange: () => void;
};

export const WorkspaceTerminalAlerts = ({
	agent,
	status,
	onAlertChange,
}: WorkspaceTerminalAlertsProps) => {
	const lifecycleState = agent?.lifecycle_state;
	const [prevLifecycleState, setPrevLifecycleState] = useState(lifecycleState);
	const [showLoadedScriptsAlert, setShowLoadedScriptsAlert] = useState(false);
	if (prevLifecycleState !== lifecycleState) {
		setShowLoadedScriptsAlert(
			prevLifecycleState === "starting" && lifecycleState === "ready",
		);
		setPrevLifecycleState(lifecycleState);
	}

	// MutationObserver triggers onAlertChange after DOM updates so
	// the terminal can refit once alert height changes.
	const wrapperRef = useRef<HTMLDivElement>(null);
	useEffect(() => {
		if (!wrapperRef.current) {
			return;
		}
		const observer = new MutationObserver(onAlertChange);
		observer.observe(wrapperRef.current, { childList: true });

		return () => {
			observer.disconnect();
		};
	}, [onAlertChange]);

	return (
		<div ref={wrapperRef}>
			{status === "disconnected" ? (
				<DisconnectedAlert />
			) : lifecycleState === "start_error" ? (
				<ErrorScriptAlert />
			) : lifecycleState === "starting" ? (
				<LoadingScriptsAlert />
			) : lifecycleState === "ready" && showLoadedScriptsAlert ? (
				<LoadedScriptsAlert />
			) : null}
		</div>
	);
};

const ErrorScriptAlert: React.FC = () => {
	return (
		<TerminalAlert
			severity="warning"
			dismissible
			actions={
				<>
					<RefreshSessionButton />
					<Link
						href={docs(
							"/admin/templates/troubleshooting#startup-script-exited-with-an-error",
						)}
						target="_blank"
						rel="noreferrer"
					>
						Go to troubleshooting docs
						<span className="sr-only"> (opens in new tab)</span>
					</Link>
				</>
			}
		>
			The startup script failed, so this workspace may be incomplete.
		</TerminalAlert>
	);
};

const LoadingScriptsAlert: React.FC = () => {
	return (
		<TerminalAlert
			dismissible
			severity="info"
			actions={<RefreshSessionButton />}
		>
			Startup scripts are still running. Some tools may not be ready yet.
		</TerminalAlert>
	);
};

const LoadedScriptsAlert: React.FC = () => {
	return (
		<TerminalAlert
			severity="success"
			dismissible
			actions={<RefreshSessionButton />}
		>
			Startup scripts finished. Refresh the session to load the updated
			environment.
		</TerminalAlert>
	);
};

const severityBorderColors: Record<AlertColor, string> = {
	info: "border-l-highlight-sky",
	success: "border-l-content-success",
	warning: "border-l-content-warning",
	error: "border-l-content-destructive",
};

const TerminalAlert: React.FC<AlertProps> = (props) => {
	const severity = props.severity ?? "info";
	return (
		<Alert
			{...props}
			className={cn(
				"rounded-none border-0 border-b border-l-[3px] border-b-border bg-surface-primary mb-px",
				severityBorderColors[severity],
			)}
		/>
	);
};

// Since the terminal connection is always trying to reconnect, we show this
// alert to indicate that the terminal is trying to connect.
const DisconnectedAlert: React.FC<AlertProps> = (props) => {
	return (
		<TerminalAlert
			{...props}
			severity="info"
			actions={<RefreshSessionButton />}
		>
			Reconnecting to your workspace.
		</TerminalAlert>
	);
};

const RefreshSessionButton: React.FC = () => {
	const [isRefreshing, setIsRefreshing] = useState(false);

	return (
		<Button
			disabled={isRefreshing}
			size="sm"
			onClick={() => {
				setIsRefreshing(true);
				location.reload();
			}}
		>
			<RefreshCwIcon className={cn(isRefreshing && "animate-spin")} />
			{isRefreshing ? "Refreshing session..." : "Refresh session"}
		</Button>
	);
};
