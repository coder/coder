export const AppPreview: React.FC<React.PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex items-center gap-2 h-8 px-3 rounded-full border border-surface-quaternary text-content-primary bg-surface-secondary shrink-0 w-fit text-xs [&>svg]:w-3.25 [&>img]:w-3.25">
			{children}
		</div>
	);
};
