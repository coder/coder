export const TableToolbar: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<div className="text-xs mb-2 mt-0 h-9 text-content-secondary flex items-center [&_strong]:text-content-primary">
			{children}
		</div>
	);
};
