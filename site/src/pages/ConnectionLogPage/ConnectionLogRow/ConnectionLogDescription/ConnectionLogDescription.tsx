import { Link as RouterLink } from "react-router";
import type { ConnectionLog } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { connectionLogMethodLabels } from "../../connectionLogMethodLabels";

type ConnectionLogDescriptionProps = {
	connectionLog: ConnectionLog;
};

export const ConnectionLogDescription: React.FC<
	ConnectionLogDescriptionProps
> = ({ connectionLog }) => {
	const {
		connection_method,
		app_name,
		app_display_name,
		workspace_owner_username,
		workspace_name,
		web_info,
	} = connectionLog;

	switch (connection_method) {
		case "port_forwarding":
		case "workspace_app": {
			if (!web_info) return null;

			const { user, slug_or_port, status_code } = web_info;
			const isPortForward = connection_method === "port_forwarding";
			const presentAction = isPortForward ? "access" : "open";
			const pastAction = isPortForward ? "accessed" : "opened";

			const target: React.ReactNode = isPortForward ? (
				<>
					port <strong>{slug_or_port}</strong>
				</>
			) : (
				<strong>{slug_or_port}</strong>
			);

			const actionText: React.ReactNode = (() => {
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

		default: {
			const methodName = connectionLogMethodLabels[connection_method];
			const appLabel = app_display_name || app_name;
			return (
				<span>
					{appLabel ? (
						<>
							{appLabel}{" "}
							<span className="text-xs text-content-secondary">
								({methodName})
							</span>
						</>
					) : (
						methodName
					)}{" "}
					session to {workspace_owner_username}'s{" "}
					<Link asChild showExternalIcon={false} className="text-base">
						<RouterLink to={`/@${workspace_owner_username}/${workspace_name}`}>
							<strong>{workspace_name}</strong>
						</RouterLink>
					</Link>{" "}
					workspace{" "}
				</span>
			);
		}
	}
};
