import { css, type Interpolation, type Theme } from "@emotion/react";
import type { FC } from "react";
import { InlineMarkdown } from "components/Markdown/Markdown";
import { Pill } from "components/Pill/Pill";
import { readableForegroundColor } from "utils/colors";

export interface ServiceBannerViewProps {
  message: string;
  backgroundColor: string;
  isPreview: boolean;
}

export const ServiceBannerView: FC<ServiceBannerViewProps> = ({
  message,
  backgroundColor,
  isPreview,
}) => {
  return (
    <div css={[styles.banner, { backgroundColor }]} className="service-banner">
      {isPreview && <Pill type="info">Preview</Pill>}
      <div
        css={[
          styles.wrapper,
          { color: readableForegroundColor(backgroundColor) },
        ]}
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
