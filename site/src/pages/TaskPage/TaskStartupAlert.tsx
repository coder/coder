import Link from "@mui/material/Link";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import type { FC } from "react";
import { docs } from "utils/docs";

type TaskStartupAlertProps = {
	agent: WorkspaceAgent;
};

export const TaskStartupAlert: FC<TaskStartupAlertProps> = ({ agent }) => {
	const lifecycleState = agent.lifecycle_state;

	if (lifecycleState === "start_error") {
		return <ErrorScriptAlert />;
	}

	if (lifecycleState === "start_timeout") {
		return <TimeoutScriptAlert />;
	}

	return null;
};

const ErrorScriptAlert: FC = () => {
	return (
		<Alert severity="warning" dismissible>
			A workspace{" "}
			<Link
				title="Startup script has exited with an error"
				href={docs(
					"/admin/templates/troubleshooting#startup-script-exited-with-an-error",
				)}
				target="_blank"
				rel="noreferrer"
			>
				startup script has exited with an error
			</Link>
			. We recommend{" "}
			<Link
				title="Debugging the startup script"
				href={docs("/admin/templates/troubleshooting#startup-script-issues")}
				target="_blank"
				rel="noreferrer"
			>
				debugging the startup script
			</Link>{" "}
			because{" "}
			<Link
				title="Your workspace may be incomplete"
				href={docs(
					"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
				)}
				target="_blank"
				rel="noreferrer"
			>
				your workspace may be incomplete
			</Link>
			.
		</Alert>
	);
};

const TimeoutScriptAlert: FC = () => {
	return (
		<Alert severity="warning" dismissible>
			A workspace{" "}
			<Link
				title="Startup script has timed out"
				href={docs(
					"/admin/templates/troubleshooting#startup-script-exited-with-an-error",
				)}
				target="_blank"
				rel="noreferrer"
			>
				startup script has timed out
			</Link>
			. We recommend{" "}
			<Link
				title="Debugging the startup script"
				href={docs("/admin/templates/troubleshooting#startup-script-issues")}
				target="_blank"
				rel="noreferrer"
			>
				debugging the startup script
			</Link>{" "}
			because{" "}
			<Link
				title="Your workspace may be incomplete"
				href={docs(
					"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
				)}
				target="_blank"
				rel="noreferrer"
			>
				your workspace may be incomplete
			</Link>
			.
		</Alert>
	);
};
