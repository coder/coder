import type { FC, PropsWithChildren } from "react";
import { CoderIcon } from "../Icons/CoderIcon";

const Language = {
	defaultMessage: (
		<>
			Welcome to <strong>Coder</strong>
		</>
	),
};

type WelcomeProps = Readonly<
	PropsWithChildren<{
		className?: string;
	}>
>;
export const Welcome: FC<WelcomeProps> = ({ children, className }) => {
	return (
		<div className={className}>
			<div className="flex justify-center pb-1">
				<CoderIcon className="w-12 h-12" />
			</div>

			<h1 className="text-center text-3xl font-normal m-0 leading-[1.1] pb-4 [&_strong]:font-semibold">
				{children || Language.defaultMessage}
			</h1>
		</div>
	);
};
