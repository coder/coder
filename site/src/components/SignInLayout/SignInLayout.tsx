import type { FC, PropsWithChildren } from "react";

export const SignInLayout: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="grow shrink h-[100vh,-webkit-fill-available] flex justify-center items-center">
			<div className="flex flex-col items-center">
				<div className="w-full max-w-[384px] flex flex-col items-center">
					{children}
				</div>
				<div className="text-xs text-content-secondary mt-6">
					{"\u00a9"} {new Date().getFullYear()} Coder Technologies, Inc.
				</div>
			</div>
		</div>
	);
};
