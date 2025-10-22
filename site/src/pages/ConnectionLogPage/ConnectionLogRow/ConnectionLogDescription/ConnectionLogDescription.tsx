import Link from "@mui/material/Link";
import type { ConnectionLog } from "api/typesGenerated";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { connectionTypeToFriendlyName } from "utils/connection";

interface ConnectionLogDescriptionProps {
	connectionLog: ConnectionLog;
}

export const ConnectionLogDescription: FC<ConnectionLogDescriptionProps> = ({
	connectionLog,
}) => {
	const { type, workspace_owner_username, workspace_name, web_info } =
		connectionLog;

	switch (type) {
		case "port_forwarding":
		case "workspace_app": {
			if (!web_info) return null;

			const { user, slug_or_port, status_code } = web_info;
			const isPortForward = type === "port_forwarding";
			const presentAction = isPortForward ? "access" : "open";
			const pastAction = isPortForward ? "accessed" : "opened";

			const target: ReactNode = isPortForward ? (
				<>
					port <strong>{slug_or_port}</strong>
				</>
			) : (
				<strong>{slug_or_port}</strong>
			);

			const actionText: ReactNode = (() => {
				if (status_code === 303) {
					return (
						<>
							was redirected attempting to {presentAction} {target}
						</>
					);
				}
				if ((status_code ?? 0) >= 400) {
					return (
						<>
							unsuccessfully attempted to {presentAction} {target}
						</>
					);
				}
				return (
					<>
						{pastAction} {target}
					</>
				);
			})();

			const isOwnWorkspace = user
				? workspace_owner_username === user.username
				: false;

			return (
				<span>
					{user ? user.username : "Unauthenticated user"} {actionText} in{" "}
					{isOwnWorkspace ? "their" : `${workspace_owner_username}'s`}{" "}
					<Link
						component={RouterLink}
						to={`/@${workspace_owner_username}/${workspace_name}`}
					>
						<strong>{workspace_name}</strong>
					</Link>{" "}
					workspace
				</span>
			);
		}

		case "reconnecting_pty":
		case "ssh":
		case "jetbrains":
		case "vscode": {
			const friendlyType = connectionTypeToFriendlyName(type);
			return (
				<span>
					{friendlyType} session to {workspace_owner_username}'s{" "}
					<Link
						component={RouterLink}
						to={`/@${workspace_owner_username}/${workspace_name}`}
					>
						<strong>{workspace_name}</strong>
					</Link>{" "}
					workspace{" "}
				</span>
			);
		}
	}
};
