/** Separates pinned chats from pinned projects in the Pinned section. */
export const PinnedProjectsSeparator: React.FC = () => (
	<div className="flex items-center gap-2 py-1.5 text-xs text-content-secondary">
		<hr aria-label="Projects" className="m-0 h-px flex-1 border-0 bg-border" />
		<span aria-hidden="true">Projects</span>
		<span className="h-px flex-1 bg-border" />
	</div>
);
