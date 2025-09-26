import type { FC, PropsWithChildren } from "react";
export const SignInLayout: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="grow basis-0 h-screen flex justify-center items-center">
			<div className="flex flex-col items-center">
				<div className="max-w-[385px] flex flex-col items-center">
					{children}
				</div>
				<div className="text-xs text-content-secondary pt-6">
					{"\u00a9"} {new Date().getFullYear()} Coder Technologies, Inc.
				</div>
			</div>
		</div>
	);
};
