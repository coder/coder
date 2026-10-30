import { Link } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";

type AgentUserLinkProps = {
	readonly username: string;
	readonly className?: string;
	readonly children: React.ReactNode;
};

/**
 * Links to an agent user's page for people who can open it. Everyone else
 * gets the same content without a link.
 */
export const AgentUserLink: React.FC<AgentUserLinkProps> = ({
	username,
	className,
	children,
}) => {
	const { permissions } = useAuthenticated();
	if (!permissions.viewAllUsers) {
		return <span className={className}>{children}</span>;
	}
	return (
		<Link
			to={`/deployment/users/${encodeURIComponent(username)}`}
			className={className}
		>
			{children}
		</Link>
	);
};
