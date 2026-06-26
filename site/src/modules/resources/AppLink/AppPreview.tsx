import type { FC, PropsWithChildren } from "react";
export const AppPreview: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex flex-row items-center gap-2 h-8 px-3 rounded-full border border-solid border-surface-quaternary text-content-primary bg-surface-secondary flex-shrink-0 w-fit text-xs [&>svg]:w-[13px] [&>img]:w-[13px]">
			{children}
		</div>
	);
};
