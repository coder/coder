import { css, type Interpolation, type Theme } from "@emotion/react";
import type { FC } from "react";
import { InlineMarkdown } from "components/Markdown/Markdown";
import { readableForegroundColor } from "utils/colors";

export interface AnnouncementBannerViewProps {
  message?: string;
  backgroundColor?: string;
}

export const AnnouncementBannerView: FC<AnnouncementBannerViewProps> = ({
  message,
  backgroundColor,
}) => {
  if (!message || !backgroundColor) {
    return null;
  }

  return (
    <div
      css={styles.banner}
      style={{ backgroundColor }}
      className="service-banner"
    >
      <div
        css={styles.wrapper}
        style={{ color: readableForegroundColor(backgroundColor) }}
      >
        <InlineMarkdown>{message}</InlineMarkdown>
      </div>
    </div>
  );
};

const styles = {
  banner: css`
    padding: 12px;
    display: flex;
    align-items: center;
  `,
  wrapper: css`
    margin-right: auto;
    margin-left: auto;
    font-weight: 400;

    & a {
      color: inherit;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;
