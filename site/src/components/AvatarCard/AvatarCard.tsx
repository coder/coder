import { type ReactNode } from "react";
import { useTheme } from "@emotion/react";

type AvatarCardProps = {
  header: ReactNode;
  imgUrl: string;
  altText: string;

  subtitle?: ReactNode;
  width?: "sm" | "md" | "lg" | "full";
};

export function AvatarCard({
  header,
  imgUrl,
  altText,
  subtitle,
  width = "full",
}: AvatarCardProps) {
  const theme = useTheme();

  return (
    <div css={{ backgroundColor: "blue" }}>
      <div>
        <span>{header}</span>
        {subtitle && <span>{subtitle}</span>}
      </div>

      <div>
        <img src={imgUrl} alt={altText} />
      </div>
    </div>
  );
}
