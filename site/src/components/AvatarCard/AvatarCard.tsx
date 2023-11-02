import { type ReactNode } from "react";
import { useTheme } from "@emotion/react";

type AvatarCardProps = {
  header: ReactNode;
  imgUrl: string;

  subtitle?: string;
  maxWidth?: number;
};

export function AvatarCard({
  header,
  imgUrl,
  subtitle,
  maxWidth,
}: AvatarCardProps) {
  const theme = useTheme();

  return (
    <div css={{ backgroundColor: "blue" }}>
      <span>{header}</span>
      {subtitle && <span>{subtitle}</span>}
    </div>
  );
}
