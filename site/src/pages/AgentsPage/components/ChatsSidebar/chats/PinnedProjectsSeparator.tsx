import { Separator } from "#/components/Separator/Separator";

/** Separates pinned chats from pinned projects in the Pinned section. */
export const PinnedProjectsSeparator: React.FC = () => (
	<div className="flex items-center gap-2 py-1.5 text-xs text-content-secondary">
		<Separator decorative={false} aria-label="Projects" className="flex-1" />
		<span aria-hidden="true">Projects</span>
		<Separator className="flex-1" />
	</div>
);
