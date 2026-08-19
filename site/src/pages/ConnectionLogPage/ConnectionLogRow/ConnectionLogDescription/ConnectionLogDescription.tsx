import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import type {
	ConnectionLog,
	ConnectionLogFileAction,
	ConnectionLogFileProtocol,
} from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { connectionTypeToFriendlyName } from "#/utils/connection";

const fileProtocolToFriendlyName = (
	protocol: ConnectionLogFileProtocol,
): string => {
	switch (protocol) {
		case "sftp":
			return "SFTP";
		case "scp":
			return "SCP";
		case "rsync":
			return "rsync";
	}
};

// fileActionToDescription phrases the file action for a sentence like
// "SFTP session <action> <path>". For SCP/rsync the path is the
// requested root, so the phrasing stays non-committal about individual
// files.
const fileActionToDescription = (action: ConnectionLogFileAction): string => {
	switch (action) {
		case "download":
			return "downloaded";
		case "upload":
			return "uploaded";
		case "bidirectional":
			// Opened for both directions at once; either may have occurred.
			return "downloaded and/or uploaded";
		case "remove":
			return "removed";
		case "rmdir":
			return "removed directory";
		case "rename":
			return "renamed";
		case "symlink":
			return "created symlink";
		case "setattr":
			return "changed attributes of";
		case "hardlink":
			return "created hard link to";
	}
};

interface ConnectionLogDescriptionProps {
	connectionLog: ConnectionLog;
}

export const ConnectionLogDescription: FC<ConnectionLogDescriptionProps> = ({
	connectionLog,
}) => {
	const {
		type,
		workspace_owner_username,
		workspace_name,
		web_info,
		file_transfer_info,
	} = connectionLog;

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
					<Link asChild showExternalIcon={false} className="text-base">
						<RouterLink to={`/@${workspace_owner_username}/${workspace_name}`}>
							<strong>{workspace_name}</strong>
						</RouterLink>
					</Link>{" "}
					workspace
				</span>
			);
		}

		case "reconnecting_pty":
		case "ssh":
		case "jetbrains":
		case "vscode":
		case "file_transfer": {
			const friendlyType = connectionTypeToFriendlyName(type);
			return (
				<span>
					{friendlyType} session to {workspace_owner_username}'s{" "}
					<Link asChild showExternalIcon={false} className="text-base">
						<RouterLink to={`/@${workspace_owner_username}/${workspace_name}`}>
							<strong>{workspace_name}</strong>
						</RouterLink>
					</Link>{" "}
					workspace{" "}
				</span>
			);
		}

		case "file_operation": {
			if (!file_transfer_info) return null;
			const { protocol, action, path, target } = file_transfer_info;
			const friendlyProtocol = fileProtocolToFriendlyName(protocol);
			const actionText = fileActionToDescription(action);
			return (
				<span>
					{friendlyProtocol} session {actionText} <strong>{path}</strong>
					{target && (
						<>
							{" "}
							→ <strong>{target}</strong>
						</>
					)}{" "}
					in {workspace_owner_username}'s{" "}
					<Link asChild showExternalIcon={false} className="text-base">
						<RouterLink to={`/@${workspace_owner_username}/${workspace_name}`}>
							<strong>{workspace_name}</strong>
						</RouterLink>
					</Link>{" "}
					workspace
				</span>
			);
		}

		case "tunnel": {
			if (!web_info) return null;
			const { user, status_code } = web_info;
			const actor = user?.username ?? "Unknown user";
			const action =
				status_code >= 400
					? "was denied a tunnel to"
					: "established a tunnel to";
			const isOwnWorkspace = workspace_owner_username === user?.username;
			return (
				<span>
					{actor} {action}{" "}
					{isOwnWorkspace ? "their" : `${workspace_owner_username}'s`}{" "}
					<Link asChild showExternalIcon={false} className="text-base">
						<RouterLink to={`/@${workspace_owner_username}/${workspace_name}`}>
							<strong>{workspace_name}</strong>
						</RouterLink>
					</Link>{" "}
					workspace
				</span>
			);
		}
	}
};
