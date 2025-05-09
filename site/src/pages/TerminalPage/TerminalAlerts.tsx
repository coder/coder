import Link from "@mui/material/Link";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Alert, type AlertProps } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { type FC, useEffect, useRef, useState } from "react";
import { docs } from "utils/docs";
import type { ConnectionStatus } from "./types";

type TerminalAlertsProps = {
	agent: WorkspaceAgent | undefined;
	status: ConnectionStatus;
	onAlertChange: () => void;
};

export const TerminalAlerts = ({
	agent,
	status,
	onAlertChange,
}: TerminalAlertsProps) => {
	const lifecycleState = agent?.lifecycle_state;
	const prevLifecycleState = useRef(lifecycleState);
	useEffect(() => {
		prevLifecycleState.current = lifecycleState;
	}, [lifecycleState]);

	// We want to observe the children of the wrapper to detect when the alert
	// changes. So the terminal page can resize itself.
	//
	// Would it be possible to just always call fit() when this component
	// re-renders instead of using an observer?
	//
	// This is a good question and the why this does not work is that the .fit()
	// needs to run after the render so in this case, I just think the mutation
	// observer is more reliable. I could use some hacky setTimeout inside of
	// useEffect to do that, I guess, but I don't think it would be any better.
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

const TerminalAlert: FC<AlertProps> = (props) => {
	return (
		<Alert
			{...props}
			css={(theme) => ({
				borderRadius: 0,
				borderWidth: 0,
				borderBottomWidth: 1,
				borderBottomColor: theme.palette.divider,
				backgroundColor: theme.palette.background.paper,
				borderLeft: `3px solid ${theme.palette[props.severity!].light}`,
				marginBottom: 1,
			})}
		/>
	);
};

const DisconnectedAlert: FC<AlertProps> = (props) => {
	return (
		<TerminalAlert
			{...props}
			severity="warning"
			actions={<RefreshSessionButton />}
		>
			Disconnected
		</TerminalAlert>
	);
};

const RefreshSessionButton: FC = () => {
	const [isRefreshing, setIsRefreshing] = useState(false);

	return (
		<Button
			disabled={isRefreshing}
			size="sm"
			variant="subtle"
			onClick={() => {
				setIsRefreshing(true);
				window.location.reload();
			}}
		>
			{isRefreshing ? "Refreshing session..." : "Refresh session"}
		</Button>
	);
};
