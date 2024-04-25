import LaunchIcon from "@mui/icons-material/LaunchOutlined";
import Link from "@mui/material/Link";
import type { FC } from "react";

interface TermsOfServiceLinkProps {
  className?: string;
  url?: string;
}

export const TermsOfServiceLink: FC<TermsOfServiceLinkProps> = ({
  className,
  url,
}) => {
  return (
    <div css={{ paddingTop: 12, fontSize: 16 }} className={className}>
      By continuing, you agree to the{" "}
      <Link
        css={{ fontWeight: 500, textWrap: "nowrap" }}
        href={url}
        target="_blank"
        rel="noreferrer"
      >
        Terms of Service&nbsp;
        <LaunchIcon css={{ fontSize: 12 }} />
      </Link>
    </div>
  );
};
