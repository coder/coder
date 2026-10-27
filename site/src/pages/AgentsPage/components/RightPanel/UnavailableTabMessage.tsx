export const UnavailableTabMessage: React.FC<{ message: string }> = ({
	message,
}) => (
	<div className="flex h-full min-h-0 items-center justify-center px-6 text-center text-xs text-content-secondary">
		{message}
	</div>
);
