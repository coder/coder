type StepFrameProps = React.PropsWithChildren<{
	/** Navigation controls shown below the step. */
	footer: React.ReactNode;
}>;

/** Frames a wizard step and its navigation controls. */
export const StepFrame: React.FC<StepFrameProps> = ({ children, footer }) => {
	return (
		<>
			<div className="p-6 border rounded-lg overflow-x-auto">{children}</div>

			<div className="flex justify-end mt-6 gap-2">{footer}</div>
		</>
	);
};
