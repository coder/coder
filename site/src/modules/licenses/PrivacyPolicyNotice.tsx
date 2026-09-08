import type { FC } from "react";

export const PrivacyPolicyNotice: FC = () => {
	return (
		<>
			The information you provide will be treated in accordance with the{" "}
			<a
				href="https://coder.com/legal/privacy-policy"
				target="_blank"
				rel="noreferrer"
				className="text-content-link hover:underline"
			>
				Coder Privacy Policy
			</a>
			.
		</>
	);
};
