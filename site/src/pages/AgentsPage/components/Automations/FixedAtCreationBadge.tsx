import { LockIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";

type FixedAtCreationBadgeProps = {
	/** Lets the section it sits in use it as a description. */
	id?: string;
	/** The choice that is fixed, such as "Schedule trigger". */
	choice: string;
};

/**
 * Names the one choice that cannot change after creation, so it does not
 * read as if the editable fields next to it were fixed too.
 */
export const FixedAtCreationBadge: React.FC<FixedAtCreationBadgeProps> = ({
	id,
	choice,
}) => (
	<Badge id={id} variant="outline" size="sm" className="text-content-secondary">
		<LockIcon aria-hidden />
		{choice}, fixed at creation
	</Badge>
);
