import type { FC, PropsWithChildren } from "react";
import { cn } from "utils/cn";

const Language = {
	defaultMessage: "Hello World",
};

type HelloWorldProps = Readonly<
	PropsWithChildren<{
		className?: string;
	}>
>;

export const HelloWorld: FC<HelloWorldProps> = ({ children, className }) => {
	return (
		<div className={cn("text-center", className)}>
			<h1 className="text-2xl font-semibold text-content-primary">
				{children || Language.defaultMessage}
			</h1>
		</div>
	);
};
