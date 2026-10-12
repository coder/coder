import { LockIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";

type FixedAtCreationBadgeProps = {
	/** Lets the radio group it sits beside use it as a description. */
	id?: string;
};

export const FixedAtCreationBadge: React.FC<FixedAtCreationBadgeProps> = ({
	id,
}) => (
	<Badge id={id} variant="outline" size="sm" className="text-content-secondary">
		<LockIcon aria-hidden />
		Fixed at creation
	</Badge>
);
