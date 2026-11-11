export const DividerWithText: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<div className="flex items-center">
			<div className="border-b-2 border-solid w-full" />
			<span className="py-1 px-4 text-xl text-content-secondary">
				{children}
			</span>
			<div className="border-b-2 border-solid w-full" />
		</div>
	);
};
