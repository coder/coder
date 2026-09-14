import type { FC, PropsWithChildren } from "react";
import { ProductLogo } from "../Icons/ProductLogo";

type WelcomeProps = Readonly<
	PropsWithChildren<{
		className?: string;
	}>
>;
export const Welcome: FC<WelcomeProps> = ({ children, className }) => {
	return (
		<div className={className}>
			<div className="flex justify-center pb-1">
				<ProductLogo />
			</div>

			<h1 className="text-3xl font-semibold m-0 flex justify-center items-center text-center leading-snug">
				{children || "Welcome to Coder"}
			</h1>
		</div>
	);
};
