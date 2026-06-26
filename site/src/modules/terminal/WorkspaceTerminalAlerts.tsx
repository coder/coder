import { RefreshCwIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import {
	Alert,
	type AlertColor,
	type AlertProps,
} from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { cn } from "#/utils/cn";
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
	const prevLifecycleState = useRef(lifecycleState);
	useEffect(() => {
		prevLifecycleState.current = lifecycleState;
	}, [lifecycleState]);

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
			) : lifecycleState === "ready" &&
				prevLifecycleState.current === "starting" ? (
				<LoadedScriptsAlert />
			) : null}
		</div>
	);
};

const ErrorScriptAlert: FC = () => {
	return (
		<TerminalAlert
			severity="warning"
			dismissible
			actions={<RefreshSessionButton />}
		>
			The workspace{" "}
			<Link
				title="startup script has exited with an error"
				href={docs(
					"/admin/templates/troubleshooting#startup-script-exited-with-an-error",
				)}
				target="_blank"
				rel="noreferrer"
				className="mx-0"
			>
				startup script has exited with an error
			</Link>
			, we recommend reloading this session and{" "}
			<Link
				title=" debugging the startup script"
				href={docs("/admin/templates/troubleshooting#startup-script-issues")}
				target="_blank"
				rel="noreferrer"
			>
				debugging the startup script
			</Link>{" "}
			because{" "}
			<Link
				title="your workspace may be incomplete."
				href={docs(
					"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
				)}
				target="_blank"
				rel="noreferrer"
			>
				your workspace may be incomplete.
			</Link>{" "}
		</TerminalAlert>
	);
};

const LoadingScriptsAlert: FC = () => {
	return (
		<TerminalAlert
			dismissible
			severity="info"
			actions={<RefreshSessionButton />}
		>
			Startup scripts are still running. You can continue using this terminal,
			but{" "}
			<Link
				title="your workspace may be incomplete."
				href={docs(
					"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
				)}
				target="_blank"
				rel="noreferrer"
			>
				{" "}
				your workspace may be incomplete.
			</Link>
		</TerminalAlert>
	);
};

const LoadedScriptsAlert: FC = () => {
	return (
		<TerminalAlert
			severity="success"
			dismissible
			actions={<RefreshSessionButton />}
		>
			Startup scripts have completed successfully. The workspace is ready but
			this{" "}
			<Link
				title="session was started before the startup scripts finished"
				href={docs(
					"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
				)}
				target="_blank"
				rel="noreferrer"
			>
				session was started before the startup script finished.
			</Link>{" "}
			To ensure your shell environment is up-to-date, we recommend reloading
			this session.
		</TerminalAlert>
	);
};

const severityBorderColors: Record<AlertColor, string> = {
	info: "border-l-highlight-sky",
	success: "border-l-content-success",
	warning: "border-l-content-warning",
	error: "border-l-content-destructive",
};

const TerminalAlert: FC<AlertProps> = (props) => {
	const severity = props.severity ?? "info";
	return (
		<Alert
			{...props}
			className={cn(
				"rounded-none border-0 border-b border-l-[3px] border-b-border-default bg-surface-primary mb-px",
				severityBorderColors[severity],
			)}
		/>
	);
};

// Since the terminal connection is always trying to reconnect, we show this
// alert to indicate that the terminal is trying to connect.
const DisconnectedAlert: FC<AlertProps> = (props) => {
	return (
		<TerminalAlert
			{...props}
			severity="info"
			actions={<RefreshSessionButton />}
		>
			Trying to connect...
		</TerminalAlert>
	);
};

const RefreshSessionButton: FC = () => {
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
