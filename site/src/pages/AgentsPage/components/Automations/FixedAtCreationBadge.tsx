import { LockIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";

/** Marks a section whose choices cannot change after creation. */
export const FixedAtCreationBadge: React.FC = () => (
	<Badge variant="outline" size="sm" className="text-content-secondary">
		<LockIcon aria-hidden />
		Fixed at creation
	</Badge>
);
