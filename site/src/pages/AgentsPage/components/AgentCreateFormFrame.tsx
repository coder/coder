/**
 * Centers the new-chat composer in the page. On mobile the composer sits at
 * the bottom of the page, after the page header.
 */
export const AgentCreateFormFrame: React.FC<React.PropsWithChildren> = ({
	children,
}) => (
	<div className="order-last flex min-h-0 flex-none items-end justify-center overflow-auto px-4 pb-4 sm:order-0 sm:h-full sm:flex-1 sm:items-center">
		<div className="mx-auto w-full max-w-3xl">{children}</div>
	</div>
);
