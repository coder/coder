import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";

export type OAuth2ClientType = "public" | "confidential";

type ClientTypeBadgeProps = {
	type: OAuth2ClientType;
};

/**
 * Public vs confidential is a classification, not a status — what the client
 * *is*, not what it's doing — so it's a Badge rather than a StatusIndicator.
 *
 * The colour is only an aid to scanning a long table; the text carries the
 * meaning on its own.
 */
export const ClientTypeBadge: FC<ClientTypeBadgeProps> = ({ type }) => {
	return type === "public" ? (
		<Badge size="sm" variant="purple">
			Public
		</Badge>
	) : (
		<Badge size="sm" variant="default">
			Confidential
		</Badge>
	);
};
