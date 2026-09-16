import type { FC } from "react";
import { Button } from "#/components/Button/Button";

const IdpSyncIcon: FC<React.SVGProps<SVGSVGElement>> = (props) => {
	return (
		<svg
			viewBox="0 0 36 36"
			fill="none"
			xmlns="http://www.w3.org/2000/svg"
			aria-hidden="true"
			{...props}
		>
			<path
				d="M5.82625 25.0438C5.20465 23.9731 4.72843 22.8245 4.41016 21.628L6.76984 18.6749C6.74313 18.2234 6.74313 17.7708 6.76984 17.3193L4.41156 14.3662C4.72929 13.1696 5.20454 12.0204 5.82484 10.949L9.58094 10.5271C9.88088 10.1892 10.2007 9.86939 10.5386 9.56944L10.9605 5.81476C12.0304 5.19741 13.1776 4.72499 14.372 4.40991L17.3252 6.7696C17.7766 6.74288 18.2293 6.74288 18.6808 6.7696L21.6339 4.41132C22.8305 4.72905 23.9796 5.20429 25.0511 5.8246L25.473 9.58069C25.8109 9.88063 26.1307 10.2004 26.4306 10.5383L30.1853 10.9602C30.8069 12.0309 31.2831 13.1796 31.6014 14.376L29.2417 17.3291C29.2684 17.7806 29.2684 18.2333 29.2417 18.6848L31.6 21.6379C31.2845 22.8341 30.8116 23.9832 30.1938 25.0551L26.4377 25.4769C26.1377 25.8149 25.8179 26.1347 25.48 26.4346L25.0581 30.1893C23.9875 30.8109 22.8388 31.2871 21.6423 31.6054L18.6892 29.2457C18.2377 29.2724 17.7851 29.2724 17.3336 29.2457L14.3805 31.604C13.1842 31.2885 12.0351 30.8156 10.9633 30.1977L10.5414 26.4416C10.2035 26.1417 9.88369 25.8219 9.58375 25.484L5.82625 25.0438Z"
				stroke="currentColor"
				strokeWidth={2}
				strokeLinecap="round"
			/>
			<path d="M18 20.16L18 15.84" stroke="currentColor" strokeWidth={2.25} />
		</svg>
	);
};

interface IdpSyncEmptyStateProps {
	title: string;
	description: string;
	ctaLabel: string;
	docsHref: string;
}

/**
 * Empty state shown on IdP sync pages when no mappings are configured yet. It
 * invites the admin to set up a mapping and links to the relevant docs.
 */
export const IdpSyncEmptyState: FC<IdpSyncEmptyStateProps> = ({
	title,
	description,
	ctaLabel,
	docsHref,
}) => {
	return (
		<div className="flex flex-col items-center justify-center gap-4 rounded-lg border border-solid border-border p-16 text-center">
			<div className="flex size-12 items-center justify-center rounded-md bg-surface-sky">
				<IdpSyncIcon className="size-9 text-highlight-sky" />
			</div>
			<div className="flex flex-col gap-2">
				<h4 className="m-0 font-semibold text-content-primary text-sm">
					{title}
				</h4>
				<p className="m-0 max-w-[420px] text-content-secondary text-sm">
					{description}
				</p>
			</div>
			<Button asChild variant="outline" size="sm">
				<a href={docsHref} target="_blank" rel="noreferrer">
					{ctaLabel}
				</a>
			</Button>
		</div>
	);
};
