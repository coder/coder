import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import { css, useTheme } from "@emotion/react";
import { type HTMLAttributes, type PropsWithChildren, type FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { DisabledBadge, EnabledBadge } from "../Badges/Badges";

export const OptionName: FC<PropsWithChildren> = ({ children }) => {
  return <span css={{ display: "block" }}>{children}</span>;
};

export const OptionDescription: FC<PropsWithChildren> = ({ children }) => {
  const theme = useTheme();

  return (
    <span
      css={{
        display: "block",
        color: theme.palette.text.secondary,
        fontSize: 14,
        marginTop: 4,
      }}
    >
      {children}
    </span>
  );
};

interface OptionValueProps {
  children?: boolean | number | string | string[] | Record<string, boolean>;
}

export const OptionValue: FC<OptionValueProps> = (props) => {
  const { children: value } = props;
  const theme = useTheme();

  if (typeof value === "boolean") {
    return value ? <EnabledBadge /> : <DisabledBadge />;
  }

  if (typeof value === "number") {
    return <span css={styles.option}>{value}</span>;
  }

  if (!value || value.length === 0) {
    return <span css={styles.option}>Not set</span>;
  }

  if (typeof value === "string") {
    return <span css={styles.option}>{value}</span>;
  }

  if (typeof value === "object" && !Array.isArray(value)) {
    return (
      <ul css={{ listStyle: "none" }}>
        {Object.entries(value)
          .sort((a, b) => a[0].localeCompare(b[0]))
          .map(([option, isEnabled]) => (
            <li
              key={option}
              css={[
                styles.option,
                !isEnabled && {
                  marginLeft: 32,
                  color: theme.palette.text.disabled,
                },
              ]}
            >
              <div
                css={{
                  display: "inline-flex",
                  alignItems: "center",
                }}
              >
                {isEnabled && (
                  <CheckCircleOutlined
                    css={(theme) => ({
                      width: 16,
                      height: 16,
                      color: theme.palette.success.light,
                      margin: "0 8px",
                    })}
                  />
                )}
                {option}
              </div>
            </li>
          ))}
      </ul>
    );
  }

  if (Array.isArray(value)) {
    return (
      <ul css={{ listStylePosition: "inside" }}>
        {value.map((item) => (
          <li key={item} css={styles.option}>
            {item}
          </li>
        ))}
      </ul>
    );
  }

  return <span css={styles.option}>{JSON.stringify(value)}</span>;
};

type OptionConfigProps = HTMLAttributes<HTMLDivElement> & { isSource: boolean };

// OptionConfig takes a isSource bool to indicate if the Option is the source of the configured value.
export const OptionConfig = ({ isSource, ...attrs }: OptionConfigProps) => {
  const theme = useTheme();
  const borderColor = isSource
    ? theme.experimental.roles.active.outline
    : theme.palette.divider;

  return (
    <div
      {...attrs}
      css={[
        {
          fontSize: 13,
          fontFamily: MONOSPACE_FONT_FAMILY,
          fontWeight: 600,
          backgroundColor: theme.palette.background.paper,
          display: "inline-flex",
          alignItems: "center",
          borderRadius: 4,
          padding: 6,
          lineHeight: 1,
          gap: 6,
          border: `1px solid ${borderColor}`,
        },
        isSource
          ? {
              "& .OptionConfigFlag": {
                background: theme.experimental.roles.active.fill,
              },
            }
          : undefined,
      ]}
    />
  );
};

export const OptionConfigFlag = (props: HTMLAttributes<HTMLDivElement>) => {
  const theme = useTheme();

  return (
    <div
      {...props}
      className="OptionConfigFlag"
      css={{
        fontSize: 10,
        fontWeight: 600,
        display: "block",
        backgroundColor: theme.palette.divider,
        lineHeight: 1,
        padding: "2px 4px",
        borderRadius: 1,
      }}
    />
  );
};

const styles = {
  option: css`
    font-size: 14px;
    font-family: ${MONOSPACE_FONT_FAMILY};
    overflow-wrap: anywhere;
    user-select: all;

    & ul {
      padding: 16px;
    }
  `,
};
